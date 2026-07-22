# Stage 1: Build Svelte 5 Frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# Stage 2: Python FastAPI Backend
FROM python:3.10-slim

WORKDIR /workspace

# Cài đặt các dependencies hệ thống (sndfile, ffmpeg)
RUN apt-get update && apt-get install -y libsndfile1 ffmpeg && rm -rf /var/lib/apt/lists/*

# Copy requirements và cài đặt
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy backend source code
COPY . .

# Copy compiled frontend static assets from Stage 1 into public/
COPY --from=frontend-builder /workspace/public ./public

EXPOSE 8000

# Chạy FastAPI
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]

