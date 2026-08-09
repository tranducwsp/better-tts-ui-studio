FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY schemas.py .
COPY main.py .

ENV CORE_PORT=8001
ENV CORE_HOST=0.0.0.0

EXPOSE 8001

CMD ["python", "main.py"]
