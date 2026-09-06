from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from routers import agent, admin, auth

app = FastAPI(title="EDR Server")

app.include_router(agent.router, tags=["agents"])
app.include_router(auth.router, prefix="/auth", tags=["auth"])
app.include_router(admin.router, prefix="/admin", tags=["admin"])

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