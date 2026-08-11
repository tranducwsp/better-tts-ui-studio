# 🌐 Gateway API Reference & Hướng Dẫn Chi Tiết Viết Engine Manifest

Tài liệu này cung cấp toàn bộ danh sách các API Endpoints của **Control Plane Gateway** và **Đặc Tả Đầy Đủ Chi Tiết Trường-Theo-Trường (Field-by-Field Spec)** của **Engine Manifest (`GET /info`)**.

---

## 📡 1. Danh Sách Chi Tiết Các Endpoints Trên Gateway

Tất cả các API làm việc với dữ liệu ứng dụng đều nằm dưới tiền tố `/api`.

### 1.1. Endpoints Kiểm Tra Hệ Thống (Public / System)

| Method | Endpoint | Mô Tả | Bảo Vệ / Rate Limit |
| :--- | :--- | :--- | :--- |
| `GET` | `/health` | Liveness probe kiểm tra ứng dụng đang chạy | Public |
| `GET` | `/ready` | Readiness probe kiểm tra kết nối DB & Engine | Public |
| `GET` | `/swagger` | Phục vụ Giao diện Swagger UI tài liệu API | Public (nếu `ENABLE_SWAGGER=true`) |
| `GET` | `/debug/pprof/*` | Go Profiler xem tình trạng bộ nhớ RAM/Goroutine | Public (nếu `ENABLE_PPROF=true`) |

---

### 1.2. Endpoints Xác Thực & Thông Tin Engine (Auth & Public API)

| Method | Endpoint | Payload Request / Query | Nội Dung Phản Hồi | Mô Tả |
| :--- | :--- | :--- | :--- | :--- |
| `GET` | `/api/info` | - | `JSON Manifest` | Trả về thông tin Engine, Models, Capabilities, UI Schema & Voice Cloning |
| `POST` | `/api/register` | `{ "username", "password", "email" }` | `{ "message", "user" }` | Đăng ký tài khoản mới (Rate Limited) |
| `POST` | `/api/login` | `{ "username", "password" }` | `{ "message", "user" }` + Set HttpOnly Cookie | Đăng nhập hệ thống, cấp Access Token & Refresh Token Cookie |
| `POST` | `/api/auth/refresh` | (Đọc Refresh Token từ Cookie) | `{ "message" }` + Set Access Token Cookie mới | Cấp lại Access Token khi hết hạn |
| `POST` | `/api/auth/logout` | (Đọc Session Cookie) | `{ "message" }` + Clear Cookies | Đăng xuất và thu hồi phiên trên server |

---

### 1.3. Endpoints Người Dùng Dành Cho Tổng Hợp & Quản Lý Giọng (Protected User APIs)

> **Yêu cầu**: Cookie Access Token hợp lệ của tài khoản đã được kích hoạt (`RequireActiveUser`).

| Method | Endpoint | Payload / Parameters | Nội Dung Phản Hồi | Mô Tả |
| :--- | :--- | :--- | :--- | :--- |
| `GET` | `/api/me` | - | `{ "user": { "id", "username", "role", "is_active" } }` | Lấy thông tin cá nhân của tài khoản đang đăng nhập |
| `GET` | `/api/voices/{model_id}` | `model_id` trên đường dẫn | `{ "voices": [...] }` | Lấy danh sách giọng đọc của Model (bao gồm giọng hệ thống & giọng clone cá nhân) |
| `POST` | `/api/jobs/init` | `{ "model_id", "total_chunks", "text_preview" }` | `{ "job_id", "status" }` | Khởi tạo Job tổng hợp văn bản nhiều đoạn |
| `POST` | `/api/synthesize/{model_id}` | `{ "job_id", "chunk_index", "text", "voice_id", "params": {...} }` | `{ "task_id", "status": "pending" }` (HTTP 202) | Gửi yêu cầu tổng hợp 1 chunk văn bản |
| `GET` | `/api/history` | `?page=1&limit=20` | `{ "jobs": [...], "total" }` | Xem lịch sử các lượt tổng hợp của bản thân |
| `GET` | `/api/history/{job_id}` | `job_id` trên đường dẫn | `{ "job": {...}, "chunks": [...] }` | Xem thông tin chi tiết một Job đã tạo |
| `POST` | `/api/clone/upload` | Multipart Form: `file` (WAV/MP3), `name`, `gender`, `accent`, `age` | `{ "voice": { "id", "name", "file_path" } }` | Tải lên file giọng đọc mẫu để tạo giọng Clone cá nhân |
| `POST` | `/api/clone/upload-temp` | Multipart Form: `file` | `{ "temp_id", "temp_path" }` | Tải âm thanh tạm để nghe thử trước khi tạo giọng |
| `GET` | `/api/clone/voices` | - | `{ "voices": [...] }` | Danh sách giọng Clone riêng do user tự tải lên |
| `DELETE` | `/api/clone/voices/{clone_id}`| `clone_id` trên đường dẫn | `{ "message": "Voice deleted" }` | Xóa giọng Clone cá nhân |
| `GET` | `/api/tasks/{task_id}` | `task_id` trên đường dẫn | `{ "task_id", "status", "progress", "error" }` | Tra cứu trạng thái xử lý của 1 Task Chunk |
| `POST` | `/api/tasks/{task_id}/cancel`| `task_id` trên đường dẫn | `{ "message": "Task cancelled" }` | Hủy bỏ một Task Chunk đang chờ trong hàng đợi |
| `GET` | `/api/tasks/{task_id}/audio` | `task_id` trên đường dẫn | Audio Binary Stream (`audio/wav`) | Tải file âm thanh kết quả sau khi Task hoàn thành |
| `GET` | `/api/stream/tasks/{task_id}`| `task_id` trên đường dẫn | Server-Sent Events (SSE Stream) | Mở kết nối SSE nhận tiến độ realtime của Task Chunk |
| `POST` | `/api/extract-text` | Multipart Form: `file` (PDF/DOCX/TXT) | `{ "text": "..." }` | Trích xuất văn bản thô từ file tài liệu (Rate & Concurrency Limited) |

---

### 1.4. Endpoints Quản Trị Hệ Thống (Admin APIs)

> **Yêu cầu**: Cookie Access Token của tài khoản có quyền `admin` (`RequireAdmin`).

| Method | Endpoint | Payload / Parameters | Mô Tả |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/internal/engine/reload` | - | Ép Gateway tải lại Manifest từ AI Engine và bắn Webhook bảo Frontend Builder prerender lại UI bundle |
| `GET` | `/api/admin/users` | `?page=1&limit=20` | Xem danh sách toàn bộ người dùng trong hệ thống |
| `POST` | `/api/admin/users/{user_id}/approve` | `user_id` trên đường dẫn | Duyệt (kích hoạt) tài khoản cho người dùng mới đăng ký |
| `GET` | `/api/admin/users/{user_id}/history` | `user_id` trên đường dẫn | Kiểm tra toàn bộ lịch sử tổng hợp của một người dùng bất kỳ |

---

## 📄 2. ĐẶC TẢ CHI TIẾT MANIFEST (`GET /info`) - CẤU TRÚC TRƯỜNG-THEO-TRƯỜNG

Engine Manifest là một đối tượng JSON phức hợp chứa 8 khối chính. Dưới đây là đặc tả chi tiết từng khối, từng trường và danh sách các tùy chọn cho phép:

```
UniversalManifest (Root)
 ├── engine_id, engine_name, version, provider
 ├── supported_modes [ EngineModeSpec ]
 ├── capabilities (EngineCapabilities)
 ├── constraints (EngineConstraints)
 ├── audio_spec (AudioSpec)
 └── ui_schema (UISchemaSpec)
```

---

### 2.1. Khối Thông Tin Gốc (Root Properties)

| Trường (Field) | Kiểu Dữ Liệu | Bắt Buộc | Giá Trị Mặc Định | Mô Tả & Các Tùy Chọn |
| :--- | :--- | :--- | :--- | :--- |
| `engine_id` | `string` | **Có** | - | Định danh duy nhất của Engine (vd: `"vits-vietnamese"`, `"xtts-v2"`, `"piper-tts"`) |
| `engine_name` | `string` | **Có** | - | Tên hiển thị trên giao diện (vd: `"VITS Neural Studio"`, `"XTTS Voice Synthesizer"`) |
| `version` | `string` | **Có** | `"1.0.0"` | Phiên bản của Engine (vd: `"1.0.0"`, `"2.1-beta"`) |
| `provider` | `string` | Không | `""` | Tên đơn vị hoặc nhóm phát triển (vd: `"DeepMind AI"`, `"Open Source Community"`) |

---

### 2.2. Khối `supported_modes` (Danh Sách Các Mô Hình / Chế Độ Xử Lý)

Mảng `supported_modes` chứa các đối tượng `EngineModeSpec` định nghĩa từng Tab chế độ xử lý trên giao diện (vd: Mode siêu nhanh, Mode tiêu chuẩn, Mode clone).

| Trường | Kiểu Dữ Liệu | Bắt Buộc | Mô Tả |
| :--- | :--- | :--- | :--- |
| `id` | `string` | **Có** | Định danh của Mode (vd: `"fast"`, `"standard"`, `"clone"`, `"express"`) |
| `name` | `string` | **Có** | Tên hiển thị của Tab Mode (vd: `"Siêu Tốc (Fast)"`, `"Chuẩn Neural"`, `"Giọng Clone"`) |
| `description` | `string` | Không | Mô tả ngắn về đặc tính của Mode |
| `capabilities` | `object` | Không | Ghi đè Capabilities riêng cho Mode này (xem Khối 2.3) |
| `audio_spec` | `object` | Không | Ghi đè AudioSpec riêng cho Mode này (xem Khối 2.5) |

---

### 2.3. Khối `capabilities` (Tính Năng Hỗ Trợ - Capabilities)

Khối này có thể khai báo ở **tầng toàn Engine** hoặc **ghi đè riêng ở tầng Mode**. Tất cả các trường kiểu `boolean` đều chấp nhận 3 giá trị: `true` (Bật), `false` (Tắt), hoặc `null/omitted` (Kế thừa).

| Trường | Kiểu Dữ Liệu | Mặc Định Nền Tảng | Mô Tả & Tác Động Giao Diện |
| :--- | :--- | :--- | :--- |
| `supports_preset_voices` | `boolean` | `true` | Bật/Tắt danh sách giọng đọc sẵn có (System Preset Voices). Nếu `true`, hiển thị Dropdown chọn giọng |
| `supports_cloning` | `boolean` | `false` | Bật/Tắt tính năng upload file âm thanh mẫu để Clone giọng ngay lập tức. Nếu `true`, hiển thị Dropzone và Waveform Trimmer |
| `supports_voice_saving` | `boolean` | `false` | Bật/Tắt tính năng lưu giọng Clone vào thư viện cá nhân lâu dài. Nếu `true`, hiển thị nút `+ Save New Voice` |
| `supports_streaming` | `boolean` | `true` | Bật/Tắt cơ chế chia nhỏ đoạn văn bản (Chunking) và phát realtime qua SSE Stream |
| `supports_speed` | `boolean` | `true` | Bật/Tắt thanh điều khiển tốc độ đọc (Speed slider/number) |
| `supports_pitch` | `boolean` | `false` | Bật/Tắt thanh điều khiển cao độ giọng đọc (Pitch slider/number) |
| `supports_emotion` | `boolean` | `false` | Bật/Tắt bộ lựa chọn cảm xúc giọng đọc (Emotion select/radio) |
| `supports_ssml` | `boolean` | `false` | Bật/Tắt chế độ nhập và giải mã thẻ cú pháp SSML |

---

### 2.4. Khối `constraints` (Ràng Buộc Kỹ Thuật & Giới Hạn)

Định nghĩa các ranh giới tham số để Frontend sinh slider và validate dữ liệu trước khi gửi request.

```json
"constraints": {
  "max_text_length": 3000,
  "speed_range": { "min": 0.5, "max": 2.0, "default": 1.0, "step": 0.1 },
  "pitch_range": { "min": -10.0, "max": 10.0, "default": 0.0, "step": 0.5 },
  "supported_emotions": ["happy", "sad", "angry", "fearful", "neutral"],
  "chunking": {
    "max_chunk_size": 1000,
    "delimiters": ["\n\n", "\n", ". ", "; ", ", "]
  }
}
```

| Trường Chi Tiết | Kiểu Dữ Liệu | Mặc Định | Mô Tả & Tùy Chọn |
| :--- | :--- | :--- | :--- |
| `max_text_length` | `integer` | `3000` | Số ký tự tối đa cho 1 request tổng hợp. Nếu văn bản dài hơn, hệ thống tự động chia chunk |
| `speed_range.min` | `float` | `0.5` | Giá trị tốc độ nhỏ nhất |
| `speed_range.max` | `float` | `2.0` | Giá trị tốc độ lớn nhất |
| `speed_range.default` | `float` | `1.0` | Giá trị tốc độ mặc định |
| `speed_range.step` | `float` | `0.1` | Bước nhảy khi trượt thanh điều khiển |
| `pitch_range.min` | `float` | `-10.0` | Giá trị cao độ nhỏ nhất |
| `pitch_range.max` | `float` | `10.0` | Giá trị cao độ lớn nhất |
| `pitch_range.default` | `float` | `0.0` | Giá trị cao độ mặc định |
| `pitch_range.step` | `float` | `0.5` | Bước nhảy cao độ |
| `supported_emotions` | `array[string]`| `[]` | Mảng danh sách các từ khóa cảm xúc được model hỗ trợ (vd: `["neutral", "happy", "sad"]`) |
| `chunking.max_chunk_size` | `integer` | `1000` | Số ký tự tối đa của mỗi Chunk đoạn văn bản nhỏ |
| `chunking.delimiters` | `array[string]`| `["\n\n", "\n", ". "]`| Ưu tiên các điểm cắt văn bản từ cao xuống thấp để phân đoạn không bị đứt câu |

---

### 2.5. Khối `audio_spec` (Thông Số Định Dạng Âm Thanh)

Quy định các chuẩn file xuất ra (Output) và chuẩn file âm thanh mẫu nhận vào (Reference Upload Input).

| Trường | Kiểu Dữ Liệu | Mặc Định | Mô Tả & Các Giá Trị Hợp Lệ |
| :--- | :--- | :--- | :--- |
| `supported_formats` | `array[string]` | `["wav"]` | Mảng định dạng xuất hỗ trợ: `"wav"`, `"mp3"`, `"flac"`, `"ogg"`, `"aac"` |
| `supported_sample_rates` | `array[int]` | `[24000]` | Mảng tần số lấy mẫu xuất: `16000`, `22050`, `24000`, `44100`, `48000` |
| `default_format` | `string` | `"wav"` | Định dạng âm thanh mặc định |
| `default_sample_rate` | `integer` | `24000` | Tần số lấy mẫu mặc định (Hz) |
| `reference_audio_formats` | `array[string]` | `["wav"]` | Mảng định dạng file âm thanh mẫu chấp nhận upload: `"wav"`, `"mp3"`, `"ogg"`, `"flac"`, `"m4a"` |
| `reference_audio_seconds` | `float` | `5.0` | Số giây đề xuất cho một clip âm thanh mẫu chuẩn |
| `max_upload_bytes` | `int64` | `104857600` (100MB) | Dung lượng file thô tối đa cho phép kéo thả vào trình duyệt |
| `max_reference_bytes` | `int64` | `10485760` (10MB) | Dung lượng tối đa của clip âm thanh sau khi cắt gửi đến Engine |

---

### 2.6. Khối `ui_schema` (Đặc Tả Chi Tiết Giao Diện Động)

Đây là khối quyết định cách trình bày trực quan các thành phần điều khiển.

```json
"ui_schema": {
  "ui_mode": "beauty",
  "model_sort": ["fast", "standard", "clone"],
  "input_panel": {
    "file_serve": true,
    "closeable": false,
    "find_mode": "expert",
    "replace_tool": true,
    "enable_chunk_box": true,
    "auto_format": [
      { "find": "\\([^)]*\\)", "replace": "" }
    ]
  },
  "option_panel": {
    "standard": {
      "notice_banner": {
        "level": "info",
        "message": "Model tiêu chuẩn sử dụng mạng Neural 24kHz."
      },
      "voice_type": "select",
      "speed_type": "slider",
      "pitch_type": "slider",
      "emotion_type": "select",
      "preset_voices": [
        {
          "id": "vi_female_1",
          "name": "Nữ Hà Nội Chuẩn",
          "gender": "female",
          "descriptions": ["Miền Bắc", "Truyền cảm"],
          "sample_url": "/samples/vi_female_1.wav"
        }
      ],
      "voice_metadata_schema": [
        {
          "key": "accent",
          "label": "Giọng Vùng Miền",
          "type": "select",
          "required": true,
          "options": ["Northern", "Southern", "Central"]
        }
      ]
    }
  }
}
```

#### 2.6.1. Chi Tiết Các Trường Trong `ui_schema`:

1. **`ui_mode`** (`string`):
   - `"beauty"` *(Mặc định)*: Bật hiệu ứng thủy tinh mờ (Glassmorphism), màu sắc hiện đại, hiệu ứng GPU blur, icon FontAwesome phong phú.
   - `"fast"`: Tắt toàn bộ GPU filter, xóa bỏ icons và fonts nặng, tối ưu trang web siêu nhẹ 0ms FCP cho thiết bị yếu hoặc mạng chậm.

2. **`model_sort`** (`array[string]`):
   - Mảng chứa danh sách các `mode_id` để quy định **thứ tự hiển thị các Tab trên thanh Menu**. Ví dụ: `["fast", "standard", "clone"]`.

3. **`input_panel`** (Cấu hình khung nhập văn bản):
   - `file_serve` (`boolean`, mặc định `true`): Cho phép/Không cho phép upload file tài liệu (PDF, DOCX, TXT) để đọc chữ.
   - `closeable` (`boolean`, mặc định `false`): Cho phép đóng khung nhập văn bản.
   - `find_mode` (`string`): `"expert"` (bộ tìm kiếm nâng cao đầy đủ tùy chọn) hoặc `"express"` (bộ tìm kiếm nhanh đơn giản).
   - `replace_tool` (`boolean`, mặc định `true`): Bật/Tắt công cụ Tìm kiếm & Thay thế.
   - `enable_chunk_box` (`boolean`, mặc định `true`): Bật/Tắt hiển thị ô xem trước phân đoạn chia nhỏ.
   - `auto_format` (`array[AutoFormatRule]`): Danh sách các quy tắc Regex chuẩn hóa văn bản tự động `[{ "find": "chuỗi_regex", "replace": "chuỗi_thay_thế" }]`.

4. **`option_panel`** (Map các tùy chọn riêng cho từng `mode_id`):
   - `notice_banner`: Cảnh báo trực quan đầu bảng điều khiển:
     - `level` (Enum): `"info"` (Xanh dương), `"success"` (Xanh lá), `"warning"` (Vàng), `"danger"` / `"error"` (Đỏ).
     - `message`: Nội dung chuỗi thông báo.
   - `voice_type` (Enum): `"select"` (Render dạng Dropdown) hoặc `"radio"` (Render dạng các nút Radio chọn nhanh).
   - `speed_type` (Enum): `"slider"` (Thanh trượt) hoặc `"number"` (Ô nhập số trực tiếp).
   - `pitch_type` (Enum): `"slider"` (Thanh trượt) hoặc `"number"` (Ô nhập số trực tiếp).
   - `emotion_type` (Enum): `"select"` (Dropdown chọn cảm xúc) hoặc `"radio"` (Radio chọn cảm xúc).
   - `preset_voices` (Array): Khai báo danh sách các giọng cố định của Model trực tiếp trong Manifest (xem Khối 2.6.2).
   - `voice_metadata_schema` (Array): Khai báo form đăng ký thuộc tính khi tạo giọng Clone mới (xem Khối 2.6.3).

---

#### 2.6.2. Chi Tiết Cấu Trúc Giọng Cố Định (`preset_voices`)

Dùng để khai báo danh sách giọng chuẩn của Model trực tiếp trong Manifest (rất tốt cho SEO và trang tĩnh):

| Trường | Kiểu Dữ Liệu | Bắt Buộc | Mô Tả |
| :--- | :--- | :--- | :--- |
| `id` | `string` | **Có** | ID duy nhất của giọng đọc (vd: `"vi_female_hn"`) |
| `name` | `string` | **Có** | Tên giọng hiển thị (vd: `"Nữ Hà Nội - Thanh Ngân"`) |
| `gender` | `string` | Không | `"female"` (Nữ) hoặc `"male"` (Nam) |
| `descriptions` | `array[string]`| Không | Các nhãn mô tả đặc trưng (vd: `["Miền Bắc", "Truyền Cảm", "Tin Tức"]`) |
| `sample_url` | `string` | Không | URL file âm thanh nghe thử mẫu (vd: `"/static/samples/hn_female.wav"`) |

---

#### 2.6.3. Chi Tiết Cấu Trúc Form Thuộc Tính Clone (`voice_metadata_schema`)

Dùng để tự động sinh các ô nhập dữ liệu (như Vùng miền, Giới tính, Phong cách) trong Modal tạo giọng Clone mới [`CreateVoiceModal`](file:///home/amoratran/server/better-tts-ui-studio/frontend/src/lib/components/CreateVoiceModal.svelte):

| Trường | Kiểu Dữ Liệu | Bắt Buộc | Mô Tả & Các Tùy Chọn |
| :--- | :--- | :--- | :--- |
| `key` | `string` | **Có** | Tên thuộc tính trong JSON (vd: `"accent"`, `"gender"`, `"age_group"`) |
| `label` | `string` | **Có** | Nhãn hiển thị trên Form (vd: `"Vùng Miền Giọng Đọc"`) |
| `type` | `string` | **Có** | Kiểu ô nhập: `"text"` (Ô nhập chữ) hoặc `"select"` (Hộp chọn Dropdown) |
| `required` | `boolean` | Không | `true` nếu bắt buộc người dùng phải điền/chọn |
| `placeholder` | `string` | Không | Chuỗi gợi ý trong ô nhập |
| `options` | `array[string]`| Không | Danh sách các tùy chọn cho kiểu `"select"` (vd: `["Northern", "Southern", "Central"]`) |

---

## 💻 3. MẪU MANIFEST CHUẨN ĐẦY ĐỦ (COMPLETE JSON MANIFEST EXAMPLE)

AI Engineer có thể copy mẫu Manifest hoàn chỉnh dưới đây để áp dụng trực tiếp cho endpoint `GET /info` trên dịch vụ Python của mình:

```json
{
  "engine_id": "neural-tts-vietnamese",
  "engine_name": "Vietnamese Neural TTS Studio",
  "version": "2.0.0",
  "provider": "AI Speech Lab",
  "supported_modes": [
    {
      "id": "fast",
      "name": "Siêu Tốc (Fast Mode)",
      "description": "Tổng hợp tiếng nói tốc độ cao dưới 200ms",
      "capabilities": {
        "supports_preset_voices": true,
        "supports_cloning": false,
        "supports_speed": true,
        "supports_pitch": false
      }
    },
    {
      "id": "standard",
      "name": "Tiêu Chuẩn (Neural High-Res)",
      "description": "Giọng đọc truyền cảm tự nhiên 24kHz",
      "capabilities": {
        "supports_preset_voices": true,
        "supports_cloning": true,
        "supports_voice_saving": true,
        "supports_speed": true,
        "supports_pitch": true,
        "supports_emotion": true
      }
    }
  ],
  "capabilities": {
    "supports_preset_voices": true,
    "supports_cloning": true,
    "supports_voice_saving": true,
    "supports_streaming": true,
    "supports_speed": true,
    "supports_pitch": true,
    "supports_emotion": true,
    "supports_ssml": false
  },
  "constraints": {
    "max_text_length": 5000,
    "speed_range": { "min": 0.5, "max": 2.0, "default": 1.0, "step": 0.1 },
    "pitch_range": { "min": -10.0, "max": 10.0, "default": 0.0, "step": 0.5 },
    "supported_emotions": ["neutral", "happy", "sad", "angry", "news"],
    "chunking": {
      "max_chunk_size": 1000,
      "delimiters": ["\n\n", "\n", ". ", "; "]
    }
  },
  "audio_spec": {
    "supported_formats": ["wav", "mp3", "flac"],
    "supported_sample_rates": [16000, 24000, 44100],
    "default_format": "wav",
    "default_sample_rate": 24000,
    "reference_audio_formats": ["wav", "mp3", "flac"],
    "reference_audio_seconds": 5.0,
    "max_upload_bytes": 104857600,
    "max_reference_bytes": 10485760
  },
  "ui_schema": {
    "ui_mode": "beauty",
    "model_sort": ["fast", "standard"],
    "input_panel": {
      "file_serve": true,
      "closeable": false,
      "find_mode": "expert",
      "replace_tool": true,
      "enable_chunk_box": true,
      "auto_format": [
        { "find": "\\([^)]*\\)", "replace": "" },
        { "find": "\\.{2,}", "replace": "." }
      ]
    },
    "option_panel": {
      "standard": {
        "notice_banner": {
          "level": "info",
          "message": "Mô hình hỗ trợ tùy chỉnh đầy đủ cảm xúc và clone giọng cá nhân."
        },
        "voice_type": "select",
        "speed_type": "slider",
        "pitch_type": "slider",
        "emotion_type": "select",
        "preset_voices": [
          {
            "id": "vi_female_hn",
            "name": "Nữ Hà Nội - Thanh Ngân",
            "gender": "female",
            "descriptions": ["Miền Bắc", "Truyền cảm", "Tin tức"],
            "sample_url": "/samples/hn_female.wav"
          },
          {
            "id": "vi_male_sg",
            "name": "Nam Sài Gòn - Minh Triết",
            "gender": "male",
            "descriptions": ["Miền Nam", "Ấm áp", "Đọc truyện"],
            "sample_url": "/samples/sg_male.wav"
          }
        ],
        "voice_metadata_schema": [
          {
            "key": "accent",
            "label": "Vùng Miền Giọng Đọc",
            "type": "select",
            "required": true,
            "options": ["Northern (Miền Bắc)", "Southern (Miền Nam)", "Central (Miền Trung)"]
          },
          {
            "key": "gender",
            "label": "Giới Tính",
            "type": "select",
            "required": true,
            "options": ["Female (Nữ)", "Male (Nam)"]
          },
          {
            "key": "style",
            "label": "Phong Cách Đọc",
            "type": "text",
            "required": false,
            "placeholder": "Ví dụ: Tin tức, Đọc truyện, Quảng cáo..."
          }
        ]
      }
    }
  }
}
```
