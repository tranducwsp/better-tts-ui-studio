#!/usr/bin/env bash
set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

echo "🚀 Launching Core TTS Mock Test Microservice on http://localhost:8001..."

if [ ! -d "venv" ]; then
    python3 -m venv venv
fi

source venv/bin/activate
pip install -q -r requirements.txt

export CORE_PORT=8001
export CORE_HOST=0.0.0.0
export MOCK_DELAY_SEC=${MOCK_DELAY_SEC:-0.0}

python main.py
