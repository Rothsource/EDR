from pydantic import BaseModel, field_serializer
from uuid import UUID
from datetime import datetime, timezone
from typing import Optional


class AgentCreate(BaseModel):
    # What a client must send to enroll a new agent.
    hostname: str
    os: str
    enrollment_token: str
    ip_address: Optional[str] = None 
    mac_address: Optional[str] = None


class AgentResponse(BaseModel):
    agent_id: UUID
    hostname: str
    os: str
    os_version: Optional[str] = None
    status: str
    created_at: datetime
    last_seen_at: Optional[datetime] = None
    ip_address: Optional[str] = None
    mac_address: Optional[str] = None

    @field_serializer("created_at", "last_seen_at") # type: ignore
    def serialize_as_utc(self, dt: Optional[datetime]) -> Optional[str]:
        if dt is None:
            return None
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)  # naive DB value IS UTC — just label it
        return dt.isoformat()

    class Config:
        from_attributes = True
        
class AgentRegisterResponse(BaseModel):
    # Returned only once, right after registration — includes the api_key.
    agent_id: UUID
    api_key: str
    
class AgentHeartbeat(BaseModel):
    agent_id: UUID
    api_key: str
    ip_address: Optional[str] = None
    mac_address: Optional[str] = None
    
class AgentActionResponse(BaseModel):
    status: str