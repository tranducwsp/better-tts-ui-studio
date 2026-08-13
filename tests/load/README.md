# k6 Load Test — AI Voice Studio

## Running with Docker (Recommended - No installation required)

```bash
# Smoke test
docker run --rm -i --network=host -v $(pwd)/tests/load:/tests/load grafana/k6 run /tests/load/smoke.js

# API test
docker run --rm -i --network=host -v $(pwd)/tests/load:/tests/load grafana/k6 run /tests/load/api.js

# Load test (10k VUs)
docker run --rm -i --network=host -v $(pwd)/tests/load:/tests/load grafana/k6 run /tests/load/load.js
```

## Option: Install k6 on the host machine

```bash
# Debian/Ubuntu
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491429A3FBA
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update
sudo apt-get install k6

# macOS
brew install k6

# Or download directly
curl -sL https://github.com/grafana/k6/releases/latest/download/k6-v0.55.0-linux-amd64.tar.gz | tar xz
```

## Quick Run

```bash
# Smoke — 1 VU, verify all endpoints
k6 run test/smoke.js

# API — 1 VU, test each endpoint
k6 run test/api.js

# Specify a different backend
k6 run -e BASE_URL=http://localhost:8000 test/smoke.js
```

## Run 10k VU Load

```bash
# Load — 10k VU, 30-minute ramp
k6 run test/load.js

# Stress — 10k VU, 5-minute ramp
k6 run -e STAGE=stress test/load.js

# Soak — 3k VU, 2 hours
k6 run -e STAGE=soak test/load.js
```

## Run Per Category

```bash
# Auth — login/register/refresh/logout
k6 run tests/load/auth.js

# Synthesize — synthesis throughput
k6 run test/synth.js

# Streaming — SSE task progress
k6 run tests/load/streaming.js
```

## Output Results to File

```bash
# JSON summary
k6 run --out json=results.json tests/load/load.js

# InfluxDB (requires Grafana)
k6 run --out influxdb=http://localhost:8086/k6 test/load.js

# k6 Cloud
k6 cloud tests/load/load.js
```

## Structure

```
test/
├── config.js        # Shared config, stages, helpers
├── smoke.js         # 1 VU, quick — verify system is up
├── api.js           # 1 VU — test each endpoint pass/fail
├── load.js          # 10k VU — user journey (login → browse → synthesize → listen)
├── auth.js          # Auth — login/register/refresh/logout
├── synth.js         # Synthesize — throughput
└── streaming.js     # SSE — task streaming
```

## Stages

| Stage | VU | Duration | Purpose |
|-------|-----|----------|----------|
| smoke | 1 | 30s | Verify system is up |
| load | 10k | 30m | Real-world load test |
| stress | 10k | 7m | Find breaking point |
| soak | 3k | 2h | Check for memory leaks |

## Custom Metrics

| Metric | Description |
|--------|-------------|
| `errors` | Overall error rate |
| `synthesis_e2e_ms` | Submit → done (ms) |
| `audio_response_bytes` | Audio size |
| `auth_errors` | Auth error rate |
| `jobs_completed` | Number of completed jobs |
| `jobs_failed` | Number of failed jobs |
| `stream_first_event_ms` | Wait for first event |
| `chunks_per_job` | Chunks per job |

## Notes

- 2 pre-created accounts `admin`/`user`, VUs alternate to avoid rate limits
- Auth rate limit: 10 requests/60s — smoke/api are unaffected, load/stress need 2 accounts
- The example engine generates a 5-10s sine wave, not reflecting real engine performance
- `streaming.js` uses polling because k6 does not yet support native SSE