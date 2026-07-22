import os
from dotenv import load_dotenv

load_dotenv()

# CPU limit configs for 2 physical cores
os.environ["OMP_NUM_THREADS"] = "2"
os.environ["MKL_NUM_THREADS"] = "2"
os.environ["OPENBLAS_NUM_THREADS"] = "2"
os.environ["VECLIB_MAXIMUM_THREADS"] = "2"
os.environ["NUMEXPR_NUM_THREADS"] = "2"

import torch
try:
    torch.set_num_threads(2)
    torch.set_num_interop_threads(2)
except RuntimeError as e:
    print(f"Warning: Could not set torch threads ({e})")
from fastapi import FastAPI
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from fastapi.middleware.cors import CORSMiddleware
import uvicorn

from core.state import custom_presets, tts
from api.router import api_router
from init_db import init_db

app = FastAPI(title="VieNeu TTS Full API")

# Cấu hình CORS để Frontend (FE) kết nối thuận tiện với Backend (BE) ở bất kỳ domain/port nào
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Mount thư mục public
app.mount("/public", StaticFiles(directory="public"), name="public")

# Tự động tạo thư mục storage nếu chưa có để tránh lỗi sập server
if not os.path.exists("storage"):
    os.makedirs("storage")
app.mount("/storage", StaticFiles(directory="storage"), name="storage")

from sqlalchemy.sql import text
from sqlalchemy.orm import Session
from fastapi import Depends, HTTPException
from core.database import get_db

@app.get("/")
async def read_index():
    return FileResponse("public/index.html")

@app.get("/health")
def health_check():
    """Liveness Probe: Kiểm tra ứng dụng có đang chạy hay không."""
    return {"status": "ok", "service": "vieneu-tts-api"}

@app.get("/ready")
def readiness_check(db: Session = Depends(get_db)):
    """Readiness Probe: Kiểm tra kết nối DB sẵn sàng nhận request trước khi K8s điều hướng traffic."""
    try:
        db.execute(text("SELECT 1"))
        return {"status": "ready", "database": "connected"}
    except Exception as e:
        raise HTTPException(status_code=503, detail=f"Database connection error: {str(e)}")

# ==========================================
# CẤU HÌNH CÁC GIỌNG CUSTOM LOCAL
# ==========================================
@app.on_event("startup")
def startup_event():
    # Khởi tạo cơ sở dữ liệu
    init_db()

# Include tất cả các endpoint API
app.include_router(api_router, prefix="/api")

if __name__ == "__main__":
    host = os.getenv("HOST", "0.0.0.0")
    port = int(os.getenv("PORT", "8000"))
    uvicorn.run("main:app", host=host, port=port, reload=True)
