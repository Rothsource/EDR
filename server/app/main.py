import asyncio
from contextlib import asynccontextmanager
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from routers import agent, admin, auth, downloads, ws
from core.ws_heartbeat import heartbeat_loop

@asynccontextmanager
async def lifespan(app: FastAPI):
    task = asyncio.create_task(heartbeat_loop())
    yield
    task.cancel()
    try:
        await task
    except asyncio.CancelledError:
        pass

app = FastAPI(title="EDR Server", lifespan=lifespan)

app.include_router(downloads.router, tags=["downloads"])
app.include_router(agent.router, tags=["agents"])
app.include_router(auth.router, prefix="/auth", tags=["auth"])
app.include_router(admin.router, prefix="/admin", tags=["admin"])
app.include_router(ws.router, tags=["agents"])

app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:5173"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

@app.get("/")
def root():
    return {"status": "running"}