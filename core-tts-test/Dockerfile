FROM python:3.10-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

ENV CORE_PORT=8001
ENV CORE_HOST=0.0.0.0
ENV MOCK_DELAY_SEC=0.2

EXPOSE 8001

CMD ["python", "main.py"]
