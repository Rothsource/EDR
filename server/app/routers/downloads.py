from fastapi import APIRouter, HTTPException
from fastapi.responses import FileResponse
import os

router = APIRouter()

# Resolve relative to this file's location, not the current working directory —
# so it works no matter where you launch uvicorn from.
BASE_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))  # .../server/app
BINARY_DIR = os.path.join(BASE_DIR, "static", "binaries")

@router.get("/download/agent/windows")
def download_agent_windows():
    path = os.path.join(BINARY_DIR, "khemstrixAgent.exe")
    if not os.path.exists(path):
        raise HTTPException(status_code=404, detail="agent binary not available")
    return FileResponse(path, filename="khemstrixAgent.exe", media_type="application/octet-stream")

@router.get("/download/agent/linux")
def download_agent_linux():
    path = os.path.join(BINARY_DIR, "khemstrixAgent")
    if not os.path.exists(path):
        raise HTTPException(status_code=404, detail="agent binary not available")
    return FileResponse(path, filename="khemstrixAgent", media_type="application/octet-stream")