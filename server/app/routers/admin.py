import secrets
from datetime import datetime, timedelta, timezone
from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.ext.asyncio import AsyncSession

from db.database import get_db
from db.models import EnrollmentToken
from schemas.token import TokenResponse

router = APIRouter()

TOKEN_VALIDITY_DURATION = timedelta(hours=1)


@router.post("/generate-token", response_model=TokenResponse)
async def generate_token(db: AsyncSession = Depends(get_db)):
    new_token = secrets.token_urlsafe(32)
    expires_at = datetime.now(timezone.utc) + TOKEN_VALIDITY_DURATION

    token_row = EnrollmentToken(
        token=new_token,
        expires_at=expires_at,
        used=False,
    )
    db.add(token_row)

    try:
        await db.commit()
    except Exception:
        await db.rollback()
        raise HTTPException(status_code=500, detail="failed to create enrollment token")

    return TokenResponse(token=new_token, expires_at=expires_at)