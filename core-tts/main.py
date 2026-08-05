import os
import uvicorn
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from config import HOST, PORT
from routes import router

app = FastAPI(
    title="Universal Core TTS Microservice",
    description="Standalone AI Inference Engine export Specification v1.0.0",
    version="1.0.0"
)

# CORS cho Control Plane nội bộ.
#
# allow_origins=["*"] kèm allow_credentials=True vừa là cấu hình mà trình duyệt từ chối thi
# hành (không được phép dùng wildcard khi có credentials), vừa cho phép trang web bất kỳ gọi
# thẳng vào engine nếu cổng này lỡ ra tới người dùng. Engine chỉ được gọi bởi core-backend
# nên danh sách nguồn là hữu hạn và khai qua ENV.
CORE_CORS_ORIGINS = [
    o.strip()
    for o in os.getenv("CORE_CORS_ORIGINS", "http://core-backend:8000").split(",")
    if o.strip()
]

app.add_middleware(
    CORSMiddleware,
    allow_origins=CORE_CORS_ORIGINS,
    allow_credentials=True,
    allow_methods=["GET", "POST", "DELETE", "OPTIONS"],
    allow_headers=["Accept", "Content-Type"],
)

app.include_router(router)

if __name__ == "__main__":
    print(f"🚀 Core TTS Microservice starting on {HOST}:{PORT}")
    # reload chỉ bật khi được yêu cầu rõ ràng: nó theo dõi cây nguồn, nên bất kỳ tệp .py ghi
    # được vào /app đều bị nạp và chạy.
    uvicorn.run(
        "main:app",
        host=HOST,
        port=PORT,
        reload=os.getenv("CORE_RELOAD", "0") == "1",
    )
