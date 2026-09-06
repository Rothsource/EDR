import secrets
from datetime import datetime, timezone
from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from db.database import get_db
from db.models import Agent, EnrollmentToken
from schemas.agent import (
    AgentCreate,
    AgentResponse,
    AgentRegisterResponse,
    AgentHeartbeat,
)

router = APIRouter()

HEARTBEAT_THRESHOLD_SECONDS = 60


@router.post("/register", response_model=AgentRegisterResponse)
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
        status="active",
        enrollment_token=payload.enrollment_token,
    )
    db.add(new_agent)

    # 5. Mark the token as used
    token_row.used = True

    await db.commit()
    await db.refresh(new_agent)

    return AgentRegisterResponse(agent_id=new_agent.agent_id, api_key=new_api_key)


@router.post("/heartbeat")
async def heartbeat(payload: AgentHeartbeat, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Agent).where(Agent.agent_id == payload.agent_id))
    agent = result.scalar_one_or_none()

    # Same generic error for "not found" and "wrong key" — avoids leaking
    # whether an agent_id exists to someone probing the endpoint.
    if agent is None or agent.api_key != payload.api_key:
        raise HTTPException(status_code=401, detail="invalid credentials")

    agent.last_seen_at = datetime.now(timezone.utc)
    await db.commit()

    return {"status": "ok"}


@router.get("/agents", response_model=list[AgentResponse])
async def list_agents(db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Agent))
    agents = result.scalars().all()
    return agents