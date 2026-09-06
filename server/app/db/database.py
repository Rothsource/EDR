from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession, async_sessionmaker
from sqlalchemy.orm import declarative_base
from config import settings

# The async engine manages a pool of connections to Postgres
engine = create_async_engine(settings.DATABASE_URL, echo=True)

# Session factory — creates new AsyncSession objects bound to the engine
AsyncSessionLocal = async_sessionmaker(
    bind=engine,
    class_=AsyncSession,
    expire_on_commit=False,
)

# Base class that your models (Agent, EnrollmentToken, Event) will inherit from
Base = declarative_base()

# Dependency to inject a DB session into FastAPI routes
async def get_db():
    async with AsyncSessionLocal() as session:
        yield session