import base64
from datetime import datetime, timezone
from typing import Optional
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy import and_, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from core.deps import get_current_user_id
from db.database import get_db
from db.models import Event
from schemas.event import EventListResponse, EventResponse

router = APIRouter()

DEFAULT_LIMIT = 50
MAX_LIMIT = 200


def _as_utc(value: datetime) -> datetime:
    # A naive datetime bound to a timestamptz column is read as server local
    # time (same trap as the WS handler). Treat naive input as UTC.
    if value.tzinfo is None:
        return value.replace(tzinfo=timezone.utc)
    return value.astimezone(timezone.utc)


def _encode_cursor(t: datetime, event_id: UUID) -> str:
    raw = f"{t.isoformat()}|{event_id}"
    return base64.urlsafe_b64encode(raw.encode()).decode()


def _decode_cursor(cursor: str) -> tuple[datetime, UUID]:
    try:
        raw = base64.urlsafe_b64decode(cursor.encode()).decode()
        time_str, id_str = raw.split("|", 1)
        return _as_utc(datetime.fromisoformat(time_str)), UUID(id_str)
    except ValueError:
        raise HTTPException(status_code=400, detail="invalid cursor")


def _to_response(e: Event) -> EventResponse:
    return EventResponse(
        event_id=e.event_id,
        time=e.time,
        class_uid=e.class_uid,
        category_uid=e.category_uid,
        activity_id=e.activity_id,
        type_uid=e.type_uid,
        severity_id=e.severity_id,
        hostname=e.hostname,
        username=e.username,
        agent_id=e.agent_id,
        data=e.data,
        schema_version=e.schema_version,
        agent_version=e.agent_version,
        host_os=e.host_os,
        host_os_version=e.host_os_version,
        ingest_source=e.ingest_source,
        created_at=e.created_at,
    )


@router.get("/events", response_model=EventListResponse)
async def list_events(
    limit: int = Query(DEFAULT_LIMIT, ge=1, le=MAX_LIMIT),
    cursor: Optional[str] = None,
    class_uid: Optional[int] = None,
    agent_id: Optional[UUID] = None,
    min_severity: Optional[int] = Query(None, ge=0, le=6),
    since: Optional[datetime] = None,
    until: Optional[datetime] = None,
    q: Optional[str] = Query(None, max_length=100),
    db: AsyncSession = Depends(get_db),
    current_user_id: str = Depends(get_current_user_id),
):
    # TODO(multi-tenancy): like GET /agents, not scoped by tenant. `users` has
    # no tenant_id yet, so there is nothing to scope by.
    stmt = select(Event).order_by(Event.time.desc(), Event.event_id.desc())

    if class_uid is not None:
        stmt = stmt.where(Event.class_uid == class_uid)
    if agent_id is not None:
        stmt = stmt.where(Event.agent_id == agent_id)
    if min_severity is not None:
        stmt = stmt.where(Event.severity_id >= min_severity)
    if since is not None:
        stmt = stmt.where(Event.time >= _as_utc(since))
    if until is not None:
        stmt = stmt.where(Event.time < _as_utc(until))
    if q and q.strip():
        pattern = f"%{q.strip()}%"
        stmt = stmt.where(or_(Event.hostname.ilike(pattern), Event.username.ilike(pattern)))

    if cursor:
        cur_time, cur_id = _decode_cursor(cursor)
        # "Strictly older than the last row I saw"; event_id breaks ties for
        # events sharing the same `time`. Stays stable while new events arrive.
        stmt = stmt.where(
            or_(
                Event.time < cur_time,
                and_(Event.time == cur_time, Event.event_id < cur_id),
            )
        )

    result = await db.execute(stmt.limit(limit + 1))  # +1 to detect a next page
    rows = result.scalars().all()

    has_more = len(rows) > limit
    rows = rows[:limit]
    next_cursor = _encode_cursor(rows[-1].time, rows[-1].event_id) if has_more else None

    return EventListResponse(items=[_to_response(r) for r in rows], next_cursor=next_cursor)