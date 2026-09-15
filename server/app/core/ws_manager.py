import asyncio
from uuid import UUID
from datetime import datetime, timezone
from fastapi import WebSocket


class ConnectionManager:
    """In-memory registry of currently-connected agents. One shared instance,
    imported by the WS route and the heartbeat loop. No DB, no network —
    just bookkeeping, so this stays cheap and easy to reason about."""

    def __init__(self) -> None:
        self._connections: dict[UUID, WebSocket] = {}
        self._last_pong: dict[UUID, datetime] = {}
        self._lock = asyncio.Lock()

    async def connect(self, agent_id: UUID, websocket: WebSocket) -> None:
        async with self._lock:
            # If this agent already has a live connection (e.g. reconnect
            # raced with a stale socket not yet cleaned up), close the old
            # one rather than silently overwriting the dict entry and
            # leaking it.
            existing = self._connections.get(agent_id)
            if existing is not None and existing is not websocket:
                try:
                    await existing.close(code=1000, reason="superseded by new connection")
                except Exception:
                    pass  # already dead, nothing to do

            self._connections[agent_id] = websocket
            self._last_pong[agent_id] = datetime.now(timezone.utc)

    async def disconnect(self, agent_id: UUID) -> None:
        async with self._lock:
            self._connections.pop(agent_id, None)
            self._last_pong.pop(agent_id, None)

    async def record_pong(self, agent_id: UUID) -> None:
        async with self._lock:
            if agent_id in self._connections:
                self._last_pong[agent_id] = datetime.now(timezone.utc)

    def get(self, agent_id: UUID) -> WebSocket | None:
        return self._connections.get(agent_id)

    def all_agent_ids(self) -> list[UUID]:
        return list(self._connections.keys())

    def last_pong(self, agent_id: UUID) -> datetime | None:
        return self._last_pong.get(agent_id)


# Single shared instance — imported by routers/ws.py and core/ws_heartbeat.py
manager = ConnectionManager()