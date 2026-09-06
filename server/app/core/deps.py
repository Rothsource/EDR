from fastapi import Header, HTTPException
from jose import JWTError

from core.security import decode_access_token

async def get_current_user_id(authorization: str = Header(None)) -> str:
    if authorization is None or not authorization.startswith("Bearer "):
        raise HTTPException(status_code=401, detail="not authenticated")

    token = authorization.removeprefix("Bearer ")
    try:
        user_id = decode_access_token(token)
    except JWTError:
        raise HTTPException(status_code=401, detail="not authenticated")

    return user_id