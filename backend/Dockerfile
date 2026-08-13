# Build stage
FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o web ./cmd/web
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o cron ./cmd/cron

# Production stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates ffmpeg

# Run as non-root user. Container root is identical to host root — if an attacker
# achieves code execution, they own everything in the container. appuser needs no special privileges:
# it only writes to /app/storage (via volume mount) and executes ffmpeg.
RUN adduser -D -u 1000 appuser

WORKDIR /app

COPY --from=builder /app/web /app/worker /app/cron .

# Do not COPY .env into the image.
#
# Previously `COPY .env* ./` bundled build-machine environment files into the image, leaking SECRET_KEY
# and initial seed passwords in pulled images. Configuration must come from runtime environment
# variables — Compose passes via `environment:`, Kubernetes via secretRef — and requireAll halts startup
# if required variables are missing, so nothing should be baked in.

USER appuser

EXPOSE 8000

# No default command: compose selects ./web, ./worker, or ./cron explicitly.
# Keeping the image commandless prevents a service from silently starting the wrong role.
