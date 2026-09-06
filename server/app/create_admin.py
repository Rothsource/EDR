import asyncio
import getpass
from db.database import AsyncSessionLocal
from db.models import User
from core.security import hash_password

async def create_admin():
    username = input("Admin username: ").strip()
    password = getpass.getpass("Admin password: ")
    confirm = getpass.getpass("Confirm password: ")

    if password != confirm:
        print("Passwords do not match. Aborting.")
        return

    async with AsyncSessionLocal() as db:
        new_user = User(username=username, password_hash=hash_password(password))
        db.add(new_user)
        await db.commit()
        await db.refresh(new_user)
        print(f"Created admin user '{username}' with user_id={new_user.user_id}")

if __name__ == "__main__":
    asyncio.run(create_admin())