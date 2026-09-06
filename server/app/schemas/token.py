from pydantic import BaseModel
from datetime import datetime


class TokenResponse(BaseModel):
    # Returned after generating a new enrollment token.
    token: str
    expires_at: datetime