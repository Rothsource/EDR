from fastapi import FastAPI
from routers import agent, admin

app = FastAPI(title="EDR Server")

app.include_router(agent.router, tags=["agents"])
app.include_router(admin.router, prefix="/admin", tags=["admin"])

@app.get("/")
def root():
    return {"status": "running"}