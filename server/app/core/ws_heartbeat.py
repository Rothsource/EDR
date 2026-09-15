import asyncio
import logging
from datetime import datetime, timezone

from schemas.ws import WSPing
from core.ws_manager import manager

logger = logging.getLogger(__name__)

PING_INTERVAL_SECONDS = 10
MISSED_PING_THRESHOLD = 3  # ~30s total before an agent is considered dead


async def _ping_all_agents() -> None:
    ping_payload = WSPing().model_dump(mode="json")

    for agent_id in manager.all_agent_ids():
        websocket = manager.get(agent_id)
        if websocket is None:
            continue  # disconnected between snapshot and now, skip

        last_pong = manager.last_pong(agent_id)
        if last_pong is not None:
            elapsed = (datetime.now(timezone.utc) - last_pong).total_seconds()
            if elapsed > PING_INTERVAL_SECONDS * MISSED_PING_THRESHOLD:
                logger.info(
                    "evicting agent %s — no pong in %.1fs (threshold %.1fs)",
                    agent_id, elapsed, PING_INTERVAL_SECONDS * MISSED_PING_THRESHOLD,
                )
                try:
                    await websocket.close(code=4408, reason="ping timeout")
                except Exception:
                    pass  # already dead, closing is best-effort
                await manager.disconnect(agent_id)
                continue

        try:
            await websocket.send_json(ping_payload)
        except Exception:
            # Send failed — socket's dead even if it hasn't been cleaned up
            # yet. Evict now rather than waiting for the next timeout cycle.
            logger.info("evicting agent %s — send failed", agent_id)
            await manager.disconnect(agent_id)


async def heartbeat_loop() -> None:
    """Background task: runs for the lifetime of the app. Pings every
    connected agent every PING_INTERVAL_SECONDS; evicts any agent that's
    missed MISSED_PING_THRESHOLD consecutive pongs."""
    while True:
        try:
            await _ping_all_agents()
        except Exception:
            # A bug in eviction/ping logic should not kill the whole
            # heartbeat loop for every other connected agent.
            logger.exception("error in heartbeat loop iteration")
        await asyncio.sleep(PING_INTERVAL_SECONDS)