#!/usr/bin/env bash
set -e

# Chuyển về thư mục gốc của core-tts-example
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"
cd "$PROJECT_ROOT"

echo "⚡ Biên dịch Protobuf (tts.proto) sang Python code..."

if [ ! -d "venv" ]; then
    echo "📦 Đang tạo virtual environment (venv)..."
    python3 -m venv venv
fi

source venv/bin/activate

echo "📦 Đang cài đặt grpcio-tools..."
pip install -q grpcio-tools

# Biên dịch tts.proto -> tts_pb2.py & tts_pb2_grpc.py
python -m grpc_tools.protoc \
    -I. \
    --python_out=. \
    --grpc_python_out=. \
    proto/tts.proto

echo "✅ Đã sinh code Python Protobuf thành công tại thư mục proto/!"
