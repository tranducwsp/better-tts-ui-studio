import os
import uvicorn
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles

from config import HOST, PORT, STORAGE_DIR
from routes import router

app = FastAPI(
    title="Universal Core TTS Microservice",
    description="Standalone AI Inference Engine export Specification v1.0.0",
    version="1.0.0"
)

# Enable CORS for internal Control Plane
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.mount("/storage", StaticFiles(directory=STORAGE_DIR), name="storage")
app.include_router(router)

if __name__ == "__main__":
    print(f"🚀 Core TTS Microservice starting on {HOST}:{PORT}")
    uvicorn.run("main:app", host=HOST, port=PORT, reload=True)
