FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY schemas.py .
COPY main.py .
COPY grpc_server.py .
COPY proto/ proto/

ENV CORE_PORT=8001
ENV CORE_HOST=0.0.0.0
ENV GRPC_PORT=50051

EXPOSE 8001
EXPOSE 50051

CMD ["python", "main.py"]
