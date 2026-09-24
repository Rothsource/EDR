import secrets
from datetime import datetime, timezone
import uuid
from venv import logger
from fastapi import APIRouter, Depends, HTTPException, Header
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from core.constants import DEFAULT_TENANT_ID


from db.database import get_db
from db.models import Agent, EnrollmentToken
from schemas.agent import (
    AgentCreate,
    AgentResponse,
    AgentRegisterResponse,
    AgentHeartbeat,
)
from core.deps import get_current_user_id  

router = APIRouter()

HEARTBEAT_THRESHOLD_SECONDS = 60

WINDOWS_AUTH_SOURCES = [f"windows-{i}" for i in (
    4624, 4625, 4634, 4647, 4648, 4672, 4778, 4779, 4800, 4801,
    4776, 4768, 4769, 4771, 4720, 4722, 4723, 4724, 4725, 4726,
    4740, 4767, 4728, 4732, 4756)]
DEFAULT_AUTH_SOURCES = {
    "linux": ["sshd", "sudo", "su"],
    "windows": WINDOWS_AUTH_SOURCES,
}


@router.post("/agent/register", response_model=AgentRegisterResponse)
async def register_agent(payload: AgentCreate, db: AsyncSession = Depends(get_db)):
    # 1. Look up the enrollment token
    result = await db.execute(
        select(EnrollmentToken).where(EnrollmentToken.token == payload.enrollment_token)
    )
    token_row = result.scalar_one_or_none()

    if token_row is None:
        raise HTTPException(status_code=400, detail="invalid token")

    # 2. Check expiry
    now = datetime.now(timezone.utc)
    expires_at = token_row.expires_at
    if expires_at.tzinfo is None:
        # stored as naive timestamp — treat as UTC for comparison
        expires_at = expires_at.replace(tzinfo=timezone.utc)

    if now > expires_at:
        raise HTTPException(status_code=400, detail="token expired")

    # 3. Check used
    if token_row.used:
        raise HTTPException(status_code=400, detail="token already used")

    # 4. Create the new agent
    new_api_key = secrets.token_urlsafe(32)

    new_agent = Agent(
        hostname=payload.hostname,
        os=payload.os,
        api_key=new_api_key,
        ip_address=payload.ip_address,
        mac_address=payload.mac_address,
        status="active",
        enrollment_token=payload.enrollment_token,
        tenant_id=DEFAULT_TENANT_ID, 
    )
    db.add(new_agent)

    # 5. Mark the token as used
    token_row.used = True

    await db.commit()
    await db.refresh(new_agent)
    return AgentRegisterResponse(agent_id=new_agent.agent_id, api_key=new_api_key)


@router.post("/agent/heartbeat")
async def heartbeat(payload: AgentHeartbeat, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Agent).where(Agent.agent_id == payload.agent_id))
    agent = result.scalar_one_or_none()

    if agent is None or agent.api_key != payload.api_key or agent.status != "active":
        raise HTTPException(status_code=401, detail="invalid credentials")

    agent.last_seen_at = datetime.now(timezone.utc).replace(tzinfo=None)

    if payload.ip_address and payload.ip_address != agent.ip_address:
        agent.ip_address = payload.ip_address
    if payload.mac_address and payload.mac_address != agent.mac_address:
        agent.mac_address = payload.mac_address

    await db.commit()

    return {
        "status": "ok",
        "config_version": agent.config_version,
    }


@router.get("/agents", response_model=list[AgentResponse])
async def list_agents(
    db: AsyncSession = Depends(get_db),
    current_user_id: str = Depends(get_current_user_id), 
):
    result = await db.execute(select(Agent))
    agents = result.scalars().all()
    return agents


@router.post("/agents/{agent_id}/rotate-key", response_model=AgentRegisterResponse)
async def rotate_agent_key(
    agent_id: str,
    db: AsyncSession = Depends(get_db),
    current_user_id: str = Depends(get_current_user_id),
):
    """
    Issues a fresh api_key for an existing agent and invalidates the old
    one immediately, in the same transaction — there is no window where
    both keys are valid. Added to revoke the two test-agent keys that
    testpush's old debug line (report item 15) wrote to plaintext logs;
    useful any time a key needs revoking for any other reason too.

    The response is the only time the new key is ever returned — same
    contract as /agent/register. After calling this, the agent's own
    identity.json on disk still has the OLD key and will start failing
    to authenticate immediately; update it by hand (or re-enroll the
    machine with a fresh token) before it's needed again.
    """
    result = await db.execute(select(Agent).where(Agent.agent_id == agent_id))
    agent = result.scalar_one_or_none()

    if agent is None:
        raise HTTPException(status_code=404, detail="agent not found")

    new_api_key = secrets.token_urlsafe(32)
    agent.api_key = new_api_key

    await db.commit()
    await db.refresh(agent)
    return AgentRegisterResponse(agent_id=agent.agent_id, api_key=new_api_key)

@router.get("/agent/config")
async def get_agent_config(
    x_agent_id: uuid.UUID | None = Header(default=None),
    x_api_key: str | None = Header(default=None),
    agent_id: uuid.UUID | None = None,   # DEPRECATED: remove once all agents are updated
    api_key: str | None = None,          # DEPRECATED
    db: AsyncSession = Depends(get_db),
):
    aid = x_agent_id or agent_id
    key = x_api_key or api_key
    if aid is None or key is None:
        raise HTTPException(status_code=401, detail="invalid credentials")
    if x_api_key is None:
        logger.warning("agent %s sent credentials in the URL (deprecated)", aid)

    result = await db.execute(select(Agent).where(Agent.agent_id == aid))
    agent = result.scalar_one_or_none()

    if (
        agent is None
        or agent.status != "active"
        or not secrets.compare_digest(agent.api_key, key)
    ):
        raise HTTPException(status_code=401, detail="invalid credentials")

    sources = sources = agent.auth_config_sources or DEFAULT_AUTH_SOURCES.get(agent.os, [])
    return {"sources": sources, "version": agent.config_version, "source": "server"}