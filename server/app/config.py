import os
from pathlib import Path
from dotenv import load_dotenv

# Explicitly point to the .env file at the project root (server/.env),
# regardless of where the script is run from
env_path = Path(__file__).resolve().parent.parent / ".env"
load_dotenv(dotenv_path=env_path)

class Settings:
    DATABASE_URL: str = os.getenv("DATABASE_URL")
    JWT_SECRET_KEY: str = os.getenv("JWT_SECRET_KEY")

    def __init__(self):
        if not self.DATABASE_URL:
            raise ValueError(f"DATABASE_URL is not set. Looked for .env at: {env_path}")
        if not self.JWT_SECRET_KEY:
            raise ValueError(f"JWT_SECRET_KEY is not set. Looked for .env at: {env_path}")

settings = Settings()