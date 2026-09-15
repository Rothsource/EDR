import asyncio
from typing import Optional
from uuid import UUID

from fastapi import APIRouter, Depends, WebSocket, WebSocketDisconnect
from pydantic import ValidationError
from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.ext.asyncio import AsyncSession

from db.database import get_db
from db.models import Agent, Event
from core.ws_manager import manager
from schemas.ws import (
    INBOUND_MESSAGE_TYPES,
    WSAuth,
    WSEvent,
    WSAck,
    WSReconcileRequest,
    WSReconcileResponse,
    WSPong,
    WSError,
)

router = APIRouter()

AUTH_TIMEOUT_SECONDS = 10


async def _send_error(websocket: WebSocket, detail: str) -> None:
    await websocket.send_json(WSError(detail=detail).model_dump(mode="json"))


async def _authenticate(websocket: WebSocket, db: AsyncSession) -> Optional[Agent]:
    """First message on the socket must be WSAuth. Same check as the REST
    heartbeat() endpoint: agent_id + api_key match + status == 'active'."""
    try:
        raw = await asyncio.wait_for(websocket.receive_json(), timeout=AUTH_TIMEOUT_SECONDS)
    except (asyncio.TimeoutError, WebSocketDisconnect, ValueError):
        await websocket.close(code=4001, reason="auth timeout or malformed first message")
        return None

    if raw.get("type") != "auth":
        await _send_error(websocket, "first message must be type 'auth'")
        await websocket.close(code=4001, reason="expected auth")
        return None

    try:
        auth_msg = WSAuth.model_validate(raw)
    except ValidationError:
        await _send_error(websocket, "malformed auth message")
        await websocket.close(code=4001, reason="malformed auth")
        return None

    result = await db.execute(select(Agent).where(Agent.agent_id == auth_msg.agent_id))
    agent = result.scalar_one_or_none()

    if agent is None or agent.api_key != auth_msg.api_key or agent.status != "active":
        await _send_error(websocket, "invalid credentials")
        await websocket.close(code=4401, reason="invalid credentials")
        return None

    return agent


async def _handle_event(db: AsyncSession, websocket: WebSocket, msg: WSEvent, agent: Agent) -> None:
    # Postgres column is TIMESTAMP WITHOUT TIME ZONE — asyncpg rejects
    # tz-aware datetimes against it, so strip tzinfo here (values are UTC).
    event_time = msg.time.replace(tzinfo=None) if msg.time.tzinfo else msg.time

    stmt = (
        pg_insert(Event)
        .values(
            event_id=msg.event_id,
            time=event_time,
            class_uid=msg.class_uid,
            category_uid=msg.category_uid,
            activity_id=msg.activity_id,
            type_uid=msg.type_uid,
            severity_id=msg.severity_id,
            hostname=msg.hostname,
            username=msg.username,
            agent_id=agent.agent_id,
            tenant_id=agent.tenant_id,
            metadata_=msg.metadata,
            data=msg.data,
        )
        .on_conflict_do_nothing(index_elements=["event_id"])
    )
    await db.execute(stmt)
    await db.commit()
    await websocket.send_json(WSAck(event_id=msg.event_id).model_dump(mode="json"))


async def _handle_reconcile(db: AsyncSession, websocket: WebSocket, msg: WSReconcileRequest) -> None:
    if not msg.event_ids:
        await websocket.send_json(WSReconcileResponse(known_event_ids=[]).model_dump(mode="json"))
        return

    result = await db.execute(select(Event.event_id).where(Event.event_id.in_(msg.event_ids)))
    known: list[UUID] = [row[0] for row in result.all()]
    await websocket.send_json(WSReconcileResponse(known_event_ids=known).model_dump(mode="json"))


@router.websocket("/agent/ws")
async def agent_ws(websocket: WebSocket, db: AsyncSession = Depends(get_db)):
    await websocket.accept()

    agent = await _authenticate(websocket, db)
    if agent is None:
        return  # already closed by _authenticate

    agent_id = agent.agent_id
    await manager.connect(agent_id, websocket)

    try:
        while True:
            try:
                raw = await websocket.receive_json()
            except ValueError:
                await _send_error(websocket, "message was not valid JSON")
                continue

            msg_type = raw.get("type")
            model_cls = INBOUND_MESSAGE_TYPES.get(msg_type)
            if model_cls is None:
                await _send_error(websocket, f"unknown message type: {msg_type!r}")
                continue

            try:
                msg = model_cls.model_validate(raw)
            except ValidationError as e:
                await _send_error(websocket, f"malformed {msg_type} message: {e}")
                continue

            if isinstance(msg, WSEvent):
                await _handle_event(db, websocket, msg, agent)
            elif isinstance(msg, WSReconcileRequest):
                await _handle_reconcile(db, websocket, msg)
            elif isinstance(msg, WSPong):
                await manager.record_pong(agent_id)
            elif isinstance(msg, WSAuth):
                # Already authenticated on this connection — a second auth
                # message is unexpected but not fatal, just ignore it.
                await _send_error(websocket, "already authenticated")

    except WebSocketDisconnect:
        pass
    finally:
        await manager.disconnect(agent_id)