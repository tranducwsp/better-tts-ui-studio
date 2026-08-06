# Build stage
FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o main main.go

# Production stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates ffmpeg

WORKDIR /app

COPY --from=builder /app/main .

# Không COPY .env vào image.
#
# Dòng `COPY .env* ./` trước đây đưa tệp môi trường của máy build vào image, nên SECRET_KEY và
# mật khẩu tài khoản khởi tạo nằm sẵn trong mọi bản pull được. Cấu hình đến từ môi trường lúc
# chạy — compose truyền qua `environment:`, k8s qua secretRef — và requireAll dừng tiến trình
# nếu thiếu biến bắt buộc, nên không có gì phải điền sẵn.

EXPOSE 8000

CMD ["./main"]
