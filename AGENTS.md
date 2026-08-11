# AGENT.md — Hướng dẫn cho coding agent

## Phạm vi

File này áp dụng cho toàn bộ repository. Nếu một thư mục có file hướng dẫn riêng trong tương lai, hướng dẫn gần file đang sửa nhất được ưu tiên.

## Mục tiêu sản phẩm

Better TTS UI Studio là **Web UI và control plane schema-driven** để AI Engineer đưa một TTS engine ra sử dụng như sản phẩm. Repository này sở hữu phần giao diện, gateway, auth, queue, storage và vận hành; không sở hữu phần model/inference.

- Giữ nền tảng độc lập với model và engine cụ thể.
- UI và gateway phải lấy model, capability, giới hạn và component động từ manifest `GET /info`.
- Không hard-code tên model, voice, mode hay tham số riêng của một engine vào luồng dùng chung.
- Sản phẩm cạnh tranh với Gradio ở lớp UI/control plane triển khai model, không cạnh tranh ở lớp huấn luyện hoặc inference model.

## Ràng buộc bắt buộc

### Không sửa Core TTS

`core-tts-example/` chỉ là stub/reference cho contract tích hợp. Core TTS thật nằm ở repository do AI Engineer quản lý.

- **Không sửa bất kỳ file nào trong `core-tts-example/`.**
- Nếu một yêu cầu cần thay đổi phía engine, hãy mô tả contract/behavior engine cần cung cấp thay vì triển khai nó tại đây.
- Thay đổi protocol hoặc manifest phải giữ tương thích giữa platform và engine ngoài repository; không tự ý phá contract hiện tại.

### Triển khai bằng Docker Compose

Workflow triển khai chuẩn của repository là `docker compose`; không build hoặc chạy các binary/image thủ công để thay thế quy trình này.

```bash
cp .env.example .env   # chỉ ở lần thiết lập đầu tiên; điền secret thật

docker compose config
docker compose up -d --build
docker compose ps
docker compose logs -f backend worker frontend-builder frontend
```

Dùng `docker compose down` khi cần dừng stack. Không dùng `down -v` trừ khi người dùng xác nhận muốn xóa dữ liệu PostgreSQL.

### Secret và cấu hình

- Không commit `.env`, secret, token, mật khẩu, credential S3 hoặc dữ liệu người dùng.
- `backend/config/settings.go` là **single source of truth** cho cấu hình backend.
- `.env.example` là file sinh tự động. Khi thay đổi setting, sửa `backend/config/settings.go`, cập nhật `LoadConfig`, rồi chạy:

  ```bash
  cd backend
  go generate ./config
  ```

- Local HTTP thường cần `COOKIE_SECURE=0`; production sau HTTPS phải giữ `COOKIE_SECURE=1`.
- Không mở Postgres, Redis, engine hoặc frontend-builder ra host nếu không có yêu cầu kiến trúc rõ ràng. Chúng là service nội bộ trong Compose.

## Cấu trúc repository

- `backend/`: Go control plane, API, auth, queue, worker, cron, storage và database.
- `frontend/`: Svelte 5 + TypeScript + Vite SPA/prerendered UI; không phải SvelteKit.
- `core-tts-example/`: stub contract chỉ đọc, không sửa.
- `k8s/`: Helm/Kubernetes deployment manifests.
- `tests/`: Test suites, fixtures, k6 smoke/load/soak tests, monitoring, và reports.
	  - `tests/load/`: k6 load test scripts.
	  - `tests/monitoring/`: Python monitoring scripts.
	  - `tests/reports/`: HTML/JSON metrics reports.
	  - `tests/fixtures/`: Shared test fixtures (dùng chung Go + frontend).
- `docs/`: tài liệu kiến trúc, backend, frontend, gateway và cấu hình.
- `docker-compose.yml`: cách chạy/deploy tích hợp chuẩn.

Đọc `README.md` và tài liệu liên quan trong `docs/` trước khi thay đổi luồng xuyên nhiều service. Kiểm tra code hiện tại nếu tài liệu và implementation mâu thuẫn; cập nhật tài liệu đã lỗi thời trong cùng thay đổi khi phù hợp.

## Quy ước backend

- Module Go nằm trong `backend/`; phiên bản Go trong `backend/go.mod` là nguồn chuẩn.
- Các process có trách nhiệm riêng:
  - `cmd/web`: HTTP API/control plane.
  - `cmd/worker`: consume Redis stream và chạy synthesis pipeline.
  - `cmd/cron`: dọn file tạm và chunk stale; chỉ chạy một replica.
- Không đưa inference dài hạn vào request handler. Web enqueue công việc, worker xử lý, client theo dõi qua SSE.
- Giữ authorization/ownership checks cho mọi tài nguyên user: job, chunk, history, voice và audio.
- Giữ abstraction `storage.Store`; code nghiệp vụ không được phụ thuộc trực tiếp vào local filesystem khi S3 cũng phải hoạt động.
- Redis có fallback in-memory cho triển khai đơn replica. Không giả định fallback này hỗ trợ semantics đa replica.
- Dùng error/JSON response helpers hiện có và giữ style comment, naming, package layout của code xung quanh.
- Chạy `gofmt` trên mọi file Go đã sửa.

### Database và code sinh tự động

- Mọi thay đổi schema runtime cần migration `up` và `down` mới trong `backend/db/migrations/`; không sửa migration đã được phát hành để thay đổi lịch sử.
- Đồng bộ `backend/db/schema.sql` với schema cuối cùng mà query/codegen cần thấy.
- SQL query nguồn nằm trong `backend/db/query/`.
- `backend/db/sqlc/` là code sinh tự động; không sửa tay. Sau khi đổi schema hoặc query, chạy từ `backend/`:

  ```bash
  sqlc generate
  ```

- `backend/proto/tts.pb.go` và `backend/proto/tts_grpc.pb.go` là generated files; không sửa tay. Chỉ thay contract protobuf khi yêu cầu đã được phối hợp với engine ngoài repository, rồi regenerate bằng toolchain protobuf tương ứng.

## Quy ước frontend

- Dùng Svelte 5 runes và TypeScript theo pattern hiện có; không đưa SvelteKit/router/framework state mới vào nếu không có lý do được duyệt.
- Giữ UI schema-driven. Component động, range, text limit, audio spec và capability phải xuất phát từ manifest hoặc helper dùng chung.
- API cần đăng nhập phải dùng `authFetch` trong `frontend/src/lib/api.ts` để giữ cookie và refresh-token retry. Không dùng `fetch` trần cho endpoint protected.
- Giữ request relative (`/api/...`, `/storage/...`) để Vite/Nginx proxy quyết định backend target.
- Khi thêm behavior thuần logic, ưu tiên tách helper TypeScript và thêm test `*.test.ts` thay vì nhúng toàn bộ logic vào component `.svelte`.
- Không sửa hoặc commit output runtime/build như `frontend/dist/`, `frontend/node_modules/` hay dữ liệu trong `backend/storage/`.

## Lệnh kiểm tra

Chạy kiểm tra nhỏ nhất liên quan trước, sau đó chạy suite đầy đủ của phần đã sửa.

### Backend

```bash
cd backend
gofmt -w <go-files-da-sua>
go test ./...
go vet ./...
go build ./cmd/...
```

`go test ./...` là kiểm tra tối thiểu cho thay đổi backend. Nếu thay config, test cũng phải xác nhận `.env.example` vẫn đồng bộ. Nếu thay query/schema, chạy `sqlc generate` trước test và kiểm tra generated diff.

### Frontend

```bash
cd frontend
npm ci
npm run check
npm test
npm run build
```

Dùng `npm ci` khi cần cài dependency từ lockfile; không thay `package-lock.json` nếu dependency không đổi.

### Tích hợp

Sau thay đổi chạm contract giữa frontend/backend, queue, storage hoặc deployment:

```bash
docker compose config
docker compose up -d --build
docker compose ps
curl -f http://localhost:8000/health
```

Smoke test k6 khi stack đã sẵn sàng:

```bash
docker run --rm -i --network=host \
  -v "$(pwd)/tests/load:/tests/load" grafana/k6 run /tests/load/smoke.js
```

Không chạy load/stress/soak test nếu người dùng chưa yêu cầu: chúng tốn tài nguyên và có thể tạo nhiều dữ liệu.

## Test layout

Mọi test file phải nằm trong thư mục `tests/` tương ứng — không đặt test file cạnh source code:

- `backend/tests/{unit,integration,contract}/` cho Go test
- `frontend/tests/unit/` cho TypeScript/vitest test
- `tests/{load,monitoring,reports,fixtures}/` cho k6, Python, reports, fixtures

Kiểm tra layout:

```bash
scripts/check-test-layout.sh
```

## Checklist trước khi hoàn tất

1. Thay đổi chỉ nằm trong phạm vi yêu cầu; không có refactor không liên quan.
2. Không file nào trong `core-tts-example/` bị sửa.
3. Không secret, `.env`, runtime data hay generated build output bị thêm vào Git.
4. Behavior mới có test hoặc có giải thích rõ vì sao chưa thể test tự động.
5. Đã chạy các lệnh kiểm tra phù hợp và báo chính xác lệnh nào pass/fail/skipped.
6. Thay đổi schema có migration; thay query/config có regenerated artifacts đúng nguồn.
7. Thay đổi API/manifest/deployment có cập nhật tài liệu liên quan khi contract đã đổi.
8. Luồng schema-driven, auth, ownership, storage abstraction và ranh giới service vẫn được giữ nguyên.
