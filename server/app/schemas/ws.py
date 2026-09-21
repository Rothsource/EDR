from pydantic import BaseModel, Field
from uuid import UUID
from datetime import datetime
from typing import Optional, Literal, Any


class WSAuth(BaseModel):
    """First message on the socket. Unauthenticated until this succeeds."""
    type: Literal["auth"] = "auth"
    agent_id: UUID
    api_key: str


class WSEvent(BaseModel):
    """Mirrors Event columns. event_id is agent-generated (idempotency key).
    agent_id/tenant_id are NOT included here — same rule as REST inserts,
    they're stamped server-side from the authenticated connection.
    ingest_source is likewise NOT included: the server sets it, never the client."""
    type: Literal["event"] = "event"
    event_id: UUID
    time: datetime
    class_uid: int
    category_uid: int
    activity_id: int
    type_uid: int
    severity_id: int
    hostname: Optional[str] = None
    username: Optional[str] = None
    metadata: Optional[dict[str, Any]] = None
    data: dict[str, Any]

    # Envelope fields added Sept 18. All optional so older agents that don't
    # send them still validate. Bounds matter: events.schema_version is a
    # smallint, and an out-of-range value would raise a DB error inside
    # _handle_event (uncaught) instead of a clean validation error here.
    schema_version: int = Field(default=1, ge=1, le=32767)
    agent_version: Optional[str] = Field(default=None, max_length=64)
    host_os: Optional[str] = Field(default=None, max_length=32)  # plain str, not an enum: new OSes shouldn't strand events
    host_os_version: Optional[str] = Field(default=None, max_length=128)


class WSAck(BaseModel):
    type: Literal["ack"] = "ack"
    event_id: UUID


class WSReconcileRequest(BaseModel):
    type: Literal["reconcile_request"] = "reconcile_request"
    event_ids: list[UUID]


class WSReconcileResponse(BaseModel):
    type: Literal["reconcile_response"] = "reconcile_response"
    known_event_ids: list[UUID]


class WSPing(BaseModel):
    type: Literal["ping"] = "ping"


class WSPong(BaseModel):
    type: Literal["pong"] = "pong"


class WSError(BaseModel):
    """Sent right before the server closes the connection on a bad message
    or failed auth — gives the agent something to log, not just a raw close code."""
    type: Literal["error"] = "error"
    detail: str


# Discriminator map used by the route to dispatch an incoming {"type": ...} payload
# to the right model. Only client->server message types belong here.
INBOUND_MESSAGE_TYPES: dict[str, type[BaseModel]] = {
    "auth": WSAuth,
    "event": WSEvent,
    "reconcile_request": WSReconcileRequest,
    "pong": WSPong,
}