import asyncio
from sqlalchemy import select
from db.database import AsyncSessionLocal
from db.models import Agent

async def test_models():
    async with AsyncSessionLocal() as session:
        result = await session.execute(select(Agent))
        agents = result.scalars().all()
        print(f"Found {len(agents)} agents")

asyncio.run(test_models())