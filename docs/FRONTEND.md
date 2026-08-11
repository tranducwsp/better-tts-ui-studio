# 🎨 Đặc Tả Kỹ Thuật Frontend (Frontend Technical Specification)

Tài liệu này chi tiết đặc tả kỹ thuật của ứng dụng **Frontend Web Studio**, bao gồm kiến trúc giao diện, công nghệ sử dụng, cơ chế tự động dựng UI từ Manifest (Dynamic Schema-Driven Rendering), xử lý âm thanh thời gian thực với Web Audio API và SSE Streaming.

---

## 🛠️ 1. Công Nghệ Sử Dụng (Tech Stack)

| Thành Phần | Công Nghệ / Thư Viện | Lý Do Lựa Chọn |
| :--- | :--- | :--- |
| **Core Framework** | **Svelte 5 (Runes)** | Sử dụng cơ chế phản ứng hạt mịn (Fine-grained reactivity) với `$state`, `$derived`, `$effect`, không Virtual DOM, mang lại hiệu năng cao và kích thước bundle cực nhỏ |
| **Build Tool** | **Vite 5** | Trình đóng gói tốc độ cao, hỗ trợ HMR (Hot Module Replacement) tức thì |
| **Ngôn Ngữ** | **TypeScript** | Type-safety cho toàn bộ dữ liệu Manifest, API payloads và Web Audio buffers |
| **Styling Engine** | **Vanilla CSS (Glassmorphism)** | CSS Variables tự định nghĩa, hiệu ứng thủy tinh mờ hiện đại, tối ưu giao diện tối (Dark Mode), không dùng Tailwind |
| **Icons & Fonts** | **FontAwesome 6 + Inter Font** | Font chữ Inter chuẩn hóa và hệ thống biểu tượng vector phong phú |
| **Audio Engine** | **Web Audio API** | Giải mã binary PCM/WAV trực tiếp trên trình duyệt, nối đoạn âm thanh phát realtime |

---

## 🏛️ 2. Kiến Trúc Components (Component Architecture)

Giao diện được thiết kế theo mô hình thành phần tập trung, chia nhỏ theo nhiệm vụ:

```
App.svelte (Root State & View Manager)
 ├── Header.svelte (Nav, Model Selector, Manifest Reload, User Profile)
 ├── AuthModal.svelte (Đăng nhập / Đăng ký Modal)
 ├── AdminModal.svelte (Quản trị duyệt người dùng Modal)
 └── Studio Main Container
      ├── TextInputPanel.svelte (Nhập liệu, Regex Rules, Tìm/Thay thế, Extract File)
      ├── GenericEnginePanel.svelte (Tự động dựng Sliders, Options từ Manifest UI Schema)
      │    └── VoiceSelect.svelte (Chọn giọng đọc hệ thống & giọng Clone cá nhân)
      │         └── CreateVoiceModal.svelte (Tạo giọng Clone mới)
      │              └── WaveformTrimmer.svelte (Xem dạng sóng & Cắt audio mẫu)
      ├── StreamingPanel.svelte (Bảng phát âm thanh Chunk realtime & SSE Progress)
      └── HistoryModal.svelte (Xem lịch sử tổng hợp âm thanh)
```

### 2.1. Chi Tiết Vai Trò Của Các Component Chính

1. **`App.svelte`**:
   - Quản lý trạng thái đăng nhập cá nhân (đọc qua `/api/me`), tải thông tin Engine Manifest (`/api/info`).
   - Quản lý các Modal hiển thị (Auth, History, Admin, Create Voice).

2. **`GenericEnginePanel.svelte`**:
   - **Thành phần lõi Schema-Driven UI**: Phân tích mảng `ui_schema.components` từ Manifest để tự động render:
     - Component `slider`: Render thanh trượt HTML5 `<input type="range">` có gắn bước nhảy (`step`), giá trị tối thiểu/tối đa và số liệu realtime.
     - Component `select`: Render dropdown chọn chế độ/preset.
     - Component `toggle`: Render công tắc bật/tắt (Switch).
     - Component `notice_banners`: Hiển thị các thông báo lưu ý trực quan từ AI Engineer.

3. **`TextInputPanel.svelte`**:
   - Khung nhập văn bản đa dòng kèm bộ đếm số từ và số ký tự thời gian thực.
   - Tích hợp bộ công cụ Tìm kiếm & Thay thế (Search & Replace) tiện lợi cho việc biên tập.
   - Cho phép tải file tài liệu (PDF, DOCX, TXT) để tự động trích xuất chữ qua API `/api/extract-text`.
   - Áp dụng các quy tắc **Regex Auto-Formatting Engine** (được khai báo từ Manifest) để làm sạch văn bản trước khi gửi tổng hợp.

4. **`StreamingPanel.svelte` & Web Audio Engine**:
   - Quản lý kết nối SSE Stream (`/api/stream/tasks/{task_id}`) để nhận thông báo tiến độ theo từng chunk.
   - Nhận tín hiệu Chunk hoàn thành -> Gọi API lấy Binary Audio -> Sử dụng Web Audio API `AudioContext.decodeAudioData()` để giải mã và ghép nối phát mịn (seamless audio playback).

5. **`WaveformTrimmer.svelte`**:
   - Component tùy chỉnh sử dụng HTML5 Canvas để vẽ hình dạng sóng âm thanh (Audio Waveform) của file mẫu được upload.
   - Cho phép người dùng kéo thả 2 mốc thời gian bắt đầu và kết thúc để cắt lấy phân đoạn âm thanh đạt chuẩn nhất cho việc Clone giọng.

---

## ⚡ 3. Cơ Chế Quản Lý Trạng Thái Vẫn Sử Dụng Svelte 5 Runes

Svelte 5 thay thế cơ chế `$:` cũ bằng hệ thống **Runes** mạnh mẽ:

- **`$state()`**: Khai báo trạng thái phản ứng.
  ```typescript
  let manifest = $state<EngineManifest | null>(null);
  let isSynthesizing = $state(false);
  let params = $state<Record<string, any>>({});
  ```
- **`$derived()`**: Tự động tính toán giá trị phụ thuộc.
  ```typescript
  let cleanText = $derived(applyRegexRules(rawText, manifest?.regex_rules));
  let charCount = $derived(rawText.length);
  ```
- **`$effect()`**: Lắng nghe sự thay đổi của state để thực thi side-effects (gửi API, cập nhật UI canvas).
  ```typescript
  $effect(() => {
    if (selectedModelId) {
      loadModelVoices(selectedModelId);
    }
  });
  ```

---

## 🔊 4. Xử Lý Âm Thanh Binary & Streaming

### 4.1. Luồng Giải Mã Âm Thanh Dạng Binary Chunks

```
 [Binary ArrayBuffer] ──> AudioContext.decodeAudioData() ──> [AudioBuffer]
                                                                  │
 [Web Audio API Node Pipeline] ◄──────────────────────────────────┘
   AudioBufferSourceNode ──> GainNode (Volume) ──> AudioDestinationNode (Speakers)
```

1. Khi từng đoạn (Chunk) âm thanh hoàn thành, Frontend tải dữ liệu dưới dạng `ArrayBuffer`.
2. Trình duyệt giải mã thành `AudioBuffer` nguyên bản thông qua `AudioContext`.
3. Đoạn âm thanh được phát nối tiếp và hiển thị thanh tiến trình trực quan trên `StreamingPanel`.

---

## 🏗️ 5. Tiến Trình Prerendering & Builder Server

Frontend tích hợp một server nhỏ bằng Node.js (`scripts/builder_server.js`):

- **Nhiệm vụ**: Lắng nghe yêu cầu HTTP POST Webhook từ Backend Gateway (`FE_BUILDER_URL/reload`).
- **Hành động**: Khi nhận được tín hiệu reload, tiến trình gọi script `prerender.js` để kết nối tới Backend, lấy thông tin `GET /api/info` mới nhất và dựng lại file HTML Static Bundle tĩnh (`dist/index.html`).
- Nhờ cơ chế này, SEO, Title, Meta Descriptions và các thẻ cấu hình ban đầu luôn chuẩn xác với Manifest hiện tại mà không làm chậm thời gian tải trang của người dùng.
