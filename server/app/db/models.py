import uuid
from sqlalchemy import Column, Text, Boolean, TIMESTAMP, Float, ForeignKey
from sqlalchemy.dialects.postgresql import UUID, JSONB
from sqlalchemy.sql import func
from sqlalchemy.orm import relationship

from db.database import Base


class EnrollmentToken(Base):
    __tablename__ = "enrollment_tokens"

    token = Column(Text, primary_key=True)
    created_at = Column(TIMESTAMP, nullable=False, server_default=func.now())
    expires_at = Column(TIMESTAMP, nullable=False)
    used = Column(Boolean, nullable=False, default=False)

    # Reverse relationship: lets you do enrollment_token.agents to see all agents that used this token
    agents = relationship("Agent", back_populates="enrollment_token_obj")


class Agent(Base):
    __tablename__ = "agents"

    agent_id = Column(UUID(as_uuid=True), primary_key=True, server_default=func.gen_random_uuid())
    hostname = Column(Text, nullable=False)
    os = Column(Text, nullable=False)
    api_key = Column(Text, nullable=False, unique=True)
    status = Column(Text, nullable=False, default="active")
    enrollment_token = Column(Text, ForeignKey("enrollment_tokens.token"))
    created_at = Column(TIMESTAMP, nullable=False, server_default=func.now())
    last_seen_at = Column(TIMESTAMP)

    # Relationship back to the EnrollmentToken this agent used
    enrollment_token_obj = relationship("EnrollmentToken", back_populates="agents")

    # Relationship to all events this agent has sent
    # events = relationship("Event", back_populates="agent")

class User(Base):
    __tablename__ = "users"

    user_id = Column(UUID(as_uuid=True), primary_key=True, server_default=func.gen_random_uuid())
    username = Column(Text, nullable=False, unique=True)
    password_hash = Column(Text, nullable=False)
    created_at = Column(TIMESTAMP, nullable=False, server_default=func.now())

# class Event(Base):
#     __tablename__ = "events"

#     event_id = Column(UUID(as_uuid=True), primary_key=True, server_default=func.gen_random_uuid())
#     agent_id = Column(UUID(as_uuid=True), ForeignKey("agents.agent_id"), nullable=False)
#     event_type = Column(Text, nullable=False)
#     timestamp = Column(TIMESTAMP, nullable=False, server_default=func.now())
#     raw_data = Column(JSONB)
#     extracted_features = Column(JSONB)
#     score = Column(Float)
#     verdict = Column(Text)

#     # Relationship back to the Agent that sent this event
#     agent = relationship("Agent", back_populates="events")