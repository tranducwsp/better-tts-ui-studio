FROM python:3.10-slim

WORKDIR /workspace

# Cài đặt các dependencies hệ thống (nếu cần cho soundfile, ffmpeg, vv)
RUN apt-get update && apt-get install -y libsndfile1 ffmpeg && rm -rf /var/lib/apt/lists/*

# Copy requirements và cài đặt
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy source code của thư viện AI (giả sử có thư mục src/)
# Trong thực tế bạn sẽ copy src/ vào /workspace/src
# COPY src/ ./src/

# Copy toàn bộ code và giao diện
COPY . .

EXPOSE 8000

# Chạy FastAPI
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
