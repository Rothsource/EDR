from pydantic import BaseModel
from uuid import UUID
from datetime import datetime
from typing import Optional


class AgentCreate(BaseModel):
    # What a client must send to enroll a new agent.
    hostname: str
    os: str
    enrollment_token: str


class AgentResponse(BaseModel):
    # What the API sends back — notice api_key is excluded.
    agent_id: UUID
    hostname: str
    os: str
    status: str
    created_at: datetime
    last_seen_at: Optional[datetime] = None

    class Config:
        from_attributes = True  # allows Pydantic to read directly from SQLAlchemy model instances
        
class AgentRegisterResponse(BaseModel):
    # Returned only once, right after registration — includes the api_key.
    agent_id: UUID
    api_key: str
    
class AgentHeartbeat(BaseModel):
    agent_id: UUID
    api_key: str
    
