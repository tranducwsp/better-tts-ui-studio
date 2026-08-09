# k6 Load Test — AI Voice Studio

## Running with Docker (Recommended - No installation required)

```bash
# Smoke test
docker run --rm -i --network=host -v $(pwd)/test:/test grafana/k6 run /test/smoke.js

# API test
docker run --rm -i --network=host -v $(pwd)/test:/test grafana/k6 run /test/api.js

# Load test (10k VUs)
docker run --rm -i --network=host -v $(pwd)/test:/test grafana/k6 run /test/load.js
```

## Option: Cài đặt k6 lên máy host

```bash
# Debian/Ubuntu
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491429A3FBA
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update
sudo apt-get install k6

# macOS
brew install k6

# Hoặc tải trực tiếp
curl -sL https://github.com/grafana/k6/releases/latest/download/k6-v0.55.0-linux-amd64.tar.gz | tar xz
```

## Chạy nhanh

```bash
# Smoke — 1 VU, verify mọi endpoint
k6 run test/smoke.js

# API — 1 VU, test từng endpoint
k6 run test/api.js

# Chỉ định backend khác
k6 run -e BASE_URL=http://192.168.1.100:8000 test/smoke.js
```

## Chạy tải 10k VU

```bash
# Load — 10k VU, ramp 30 phút
k6 run test/load.js

# Stress — 10k VU, ramp 5 phút
k6 run -e STAGE=stress test/load.js

# Soak — 3k VU, 2 giờ
k6 run -e STAGE=soak test/load.js
```

## Chạy từng chuyên mục

```bash
# Auth — login/register/refresh/logout
k6 run test/auth.js

# Synthesize — throughput tổng hợp
k6 run test/synth.js

# Streaming — SSE task progress
k6 run test/streaming.js
```

## Kết quả ra file

```bash
# JSON summary
k6 run --out json=results.json test/load.js

# InfluxDB (cần Grafana)
k6 run --out influxdb=http://localhost:8086/k6 test/load.js

# k6 Cloud
k6 cloud test/load.js
```

## Cấu trúc

```
test/
├── config.js        # Shared config, stages, helpers
├── smoke.js         # 1 VU, nhanh — verify hệ thống lên
├── api.js           # 1 VU — test từng endpoint đúng/sai
├── load.js          # 10k VU — user journey (login → duyệt → tổng hợp → nghe)
├── auth.js          # Auth — login/register/refresh/logout
├── synth.js         # Synthesize — throughput
└── streaming.js     # SSE — task streaming
```

## Stages

| Stage | VU | Thời gian | Mục đích |
|-------|-----|-----------|----------|
| smoke | 1 | 30s | Verify hệ thống lên |
| load | 10k | 30m | Kiểm tra tải thực tế |
| stress | 10k | 7m | Tìm điểm gãy |
| soak | 3k | 2h | Kiểm tra memory leak |

## Custom metrics

| Metric | Mô tả |
|--------|--------|
| `errors` | Tỷ lệ lỗi tổng |
| `synthesis_e2e_ms` | Submit → done (ms) |
| `audio_response_bytes` | Kích thước audio |
| `auth_errors` | Tỷ lệ lỗi auth |
| `jobs_completed` | Số job hoàn thành |
| `jobs_failed` | Số job thất bại |
| `stream_first_event_ms` | Đợi first event |
| `chunks_per_job` | Chunks mỗi job |

## Chú ý

- 2 tài khoản `admin`/`user` tạo sẵn, VU luân phiên để tránh rate limit
- Rate limit auth: 10 requests/60s — smoke/api không bị, load/stress cần 2 tài khoản
- Engine example sinh sóng sin 5-10s, không phản ánh hiệu năng engine thật
- `streaming.js` dùng polling vì k6 chưa hỗ trợ SSE native
