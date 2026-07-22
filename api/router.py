from fastapi import APIRouter

from api.tts_standard import router as standard_router
from api.tts_clone import router as clone_router
from api.tts_fast import router as fast_router
from api.tasks import router as tasks_router
from api.utils import router as utils_router
from api.history import router as history_router
from api.auth import router as auth_router

api_router = APIRouter()

api_router.include_router(auth_router, tags=["Auth"])
api_router.include_router(history_router, tags=["History"])

# API 1 & 2: TTS Standard
api_router.include_router(standard_router, prefix="/standard", tags=["Standard TTS"])

# API 3 & 4: TTS Clone
api_router.include_router(clone_router, prefix="/clone", tags=["Clone TTS"])

# API 9: Fast TTS (Edge TTS)
api_router.include_router(fast_router, prefix="/fasttts", tags=["Fast TTS"])

# API 5, 6, 7 & Stream: Quản lý Task
api_router.include_router(tasks_router, tags=["Tasks"])

# API 8: Utils (Extract Text)
api_router.include_router(utils_router, tags=["Utils"])
