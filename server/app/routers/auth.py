from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from db.database import get_db
from db.models import User
from schemas.auth import LoginRequest, TokenResponse
from core.security import verify_password, create_access_token
from schemas.auth import LoginRequest, TokenResponse, ChangePasswordRequest 
from core.security import verify_password, create_access_token, hash_password
from core.deps import get_current_user_id

router = APIRouter()

@router.post("/login", response_model=TokenResponse)
async def login(payload: LoginRequest, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(User).where(User.username == payload.username))
    user = result.scalar_one_or_none()

    if user is None or not verify_password(payload.password, user.password_hash):
        raise HTTPException(status_code=401, detail="invalid credentials")

    token = create_access_token(user_id=user.user_id)
    return TokenResponse(access_token=token)

@router.put("/change-password")
async def change_password(
    payload: ChangePasswordRequest,
    db: AsyncSession = Depends(get_db),
    current_user_id: str = Depends(get_current_user_id),
):
    result = await db.execute(select(User).where(User.user_id == current_user_id))
    user = result.scalar_one_or_none()

    # Should never actually be None here (a valid JWT implies the user existed when issued),
    # but a deleted-user edge case is still possible — fail safely instead of crashing.
    if user is None:
        raise HTTPException(status_code=401, detail="not authenticated")

    if not verify_password(payload.current_password, user.password_hash):
        raise HTTPException(status_code=401, detail="current password is incorrect")

    user.password_hash = hash_password(payload.new_password)

    try:
        await db.commit()
    except Exception:
        await db.rollback()
        raise HTTPException(status_code=500, detail="failed to update password")

    return {"status": "password updated"}