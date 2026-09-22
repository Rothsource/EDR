from datetime import datetime
from typing import Any, Optional
from uuid import UUID

from pydantic import BaseModel


class EventResponse(BaseModel):
    event_id: UUID
    time: datetime  # UTC, stamped by the agent
    class_uid: int
    category_uid: int
    activity_id: int
    type_uid: int
    severity_id: int

    hostname: Optional[str] = None
    username: Optional[str] = None
    agent_id: Optional[UUID] = None

    data: Any  # class-specific fields; shape depends on class_uid

    schema_version: int
    agent_version: Optional[str] = None
    host_os: Optional[str] = None
    host_os_version: Optional[str] = None
    ingest_source: str

    created_at: datetime  # UTC, stamped by the server on insert


class EventListResponse(BaseModel):
    items: list[EventResponse]
    next_cursor: Optional[str] = None  # None = no more pages