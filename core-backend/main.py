import os
from dotenv import load_dotenv

load_dotenv()

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
import uvicorn

from core.state import custom_presets
from api.router import api_router
from init_db import init_db

app = FastAPI(title="VieNeu TTS Control Plane API")

# Cấu hình CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Tự động tạo thư mục storage nếu chưa có
if not os.path.exists("storage"):
    os.makedirs("storage")

from sqlalchemy.sql import text
from sqlalchemy.orm import Session
from fastapi import Depends, HTTPException
from core.database import get_db

@app.get("/health")
def health_check():
    return {"status": "ok", "service": "vieneu-core-backend"}

@app.get("/ready")
def readiness_check(db: Session = Depends(get_db)):
    try:
        db.execute(text("SELECT 1"))
        return {"status": "ready", "database": "connected"}
    except Exception as e:
        raise HTTPException(status_code=503, detail=f"Database connection error: {str(e)}")

@app.on_event("startup")
def startup_event():
    init_db()

# Include các endpoint API
app.include_router(api_router, prefix="/api")

if __name__ == "__main__":
    host = os.getenv("HOST", "0.0.0.0")
    port = int(os.getenv("PORT", "8000"))
    uvicorn.run("main:app", host=host, port=port, reload=True)
