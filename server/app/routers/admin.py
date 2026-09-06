from datetime import datetime, timedelta, timezone
import secrets

from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from uuid import UUID

from db.database import get_db
from db.models import EnrollmentToken, Agent
from schemas.token import TokenResponse
from schemas.agent import AgentActionResponse
from core.deps import get_current_user_id
from core.deps import get_current_user_id

router = APIRouter()

TOKEN_VALIDITY_DURATION = timedelta(hours=1)


@router.post("/generate-token", response_model=TokenResponse)
async def generate_token(
    db: AsyncSession = Depends(get_db),
    current_user_id: str = Depends(get_current_user_id)
):
    new_token = secrets.token_urlsafe(32)

    expires_at = datetime.now(timezone.utc) + TOKEN_VALIDITY_DURATION
    expires_at_naive = expires_at.replace(tzinfo=None)  # strip tz info before storing

    token_row = EnrollmentToken(
        token=new_token,
        expires_at=expires_at_naive,   # store the naive version
        used=False,
    )
    db.add(token_row)

    try:
        await db.commit()
    except Exception:
        await db.rollback()
        raise HTTPException(status_code=500, detail="failed to create enrollment token")

    return TokenResponse(token=new_token, expires_at=expires_at)

@router.patch("/agents/{agent_id}/revoke", response_model=AgentActionResponse)
async def revoke_agent(
    agent_id: UUID,
    db: AsyncSession = Depends(get_db),
    current_user_id: str = Depends(get_current_user_id),
):
    result = await db.execute(select(Agent).where(Agent.agent_id == agent_id))
    agent = result.scalar_one_or_none()

    if agent is None:
        raise HTTPException(status_code=404, detail="agent not found")

    agent.status = "revoked"

    try:
        await db.commit()
    except Exception:
        await db.rollback()
        raise HTTPException(status_code=500, detail="failed to revoke agent")

    return AgentActionResponse(status="revoked")


@router.delete("/agents/{agent_id}", response_model=AgentActionResponse)
async def delete_agent(
    agent_id: UUID,
    db: AsyncSession = Depends(get_db),
    current_user_id: str = Depends(get_current_user_id),
):
    result = await db.execute(select(Agent).where(Agent.agent_id == agent_id))
    agent = result.scalar_one_or_none()

    if agent is None:
        raise HTTPException(status_code=404, detail="agent not found")

    await db.delete(agent)

    try:
        await db.commit()
    except Exception:
        await db.rollback()
        raise HTTPException(status_code=500, detail="failed to delete agent")

    return AgentActionResponse(status="deleted")

@router.patch("/agents/{agent_id}/unrevoke", response_model=AgentActionResponse)
async def unrevoke_agent(
    agent_id: UUID,
    db: AsyncSession = Depends(get_db),
    current_user_id: str = Depends(get_current_user_id),
):
    result = await db.execute(select(Agent).where(Agent.agent_id == agent_id))
    agent = result.scalar_one_or_none()

    if agent is None:
        raise HTTPException(status_code=404, detail="agent not found")

    agent.status = "active"

    try:
        await db.commit()
    except Exception:
        await db.rollback()
        raise HTTPException(status_code=500, detail="failed to unrevoke agent")

    return AgentActionResponse(status="active")