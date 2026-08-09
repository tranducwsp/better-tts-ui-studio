# Công Nghệ Và Kĩ Thuật Frontend (Frontend Technical & Engineering Spec)

Tài liệu này chi tiết hóa các công nghệ, giải pháp kỹ thuật, kiến trúc mã nguồn và quy trình xây dựng ứng dụng Frontend của **Better TTS UI Studio**.

---

## 1. Công Nghệ Cốt Lõi (Tech Stack)

| Thành phần | Công nghệ / Thư viện | Phiên bản | Vai trò & Lý do chọn |
| :--- | :--- | :--- | :--- |
| **Framework** | **Svelte** | `^5.56.4` | Tận dụng Svelte 5 với cơ chế **Runes** giúp phản xạ dữ liệu (reactivity) siêu nhanh, dung lượng bundle cực nhỏ, không dùng Virtual DOM. |
| **Ngôn ngữ** | **TypeScript** | `~6.0.2` | Đảm bảo tính an toàn dữ liệu (Type-safety) cho toàn bộ API payload, Audio data structure và Component props. |
| **Build Tool** | **Vite** | `^8.1.1` | Tốc độ HMR (Hot Module Replacement) tức thì khi dev và tối ưu hóa Rollup bundler khi build production. |
| **Styling** | **Vanilla CSS (Custom Properties)** | Native CSS3 | Không dùng TailwindCSS hay UI Library nặng. Tự định nghĩa hệ thống **Design Tokens** (Variables, HSL color system, Glassmorphism). |
| **Icon & Font** | **FontAwesome Free & Inter Font** | `v7` / `v5` | Bundled hoàn toàn **Offline** (không phụ thuộc CDN ngoài), đảm bảo chạy tốt trong môi trường nội bộ/air-gapped. |
| **CSS Optimization** | **PurgeCSS** | `^8.0.0` | Loại bỏ 100% CSS không sử dụng trong quá trình build sản phẩm. |

---

## 2. Các Giải Pháp Kỹ Thuật Nổi Bật (Engineering Highlights)

### 2.1. Kiến Trúc Quản Lý Trạng Thái Với Svelte 5 Runes
Ứng dụng sử dụng triệt để các **Runes** mới của Svelte 5 thay thế cho cơ chế `$store` truyền thống:
- `$state(...)`: Quản lý reactive state cục bộ và toàn cục.
- `$derived(...)`: Tự động tính toán các giá trị phụ thuộc (đếm từ, đếm ký tự, filter danh sách voice) mà không gây re-render thừa.
- `*.svelte.ts`: Tạo các Universal Reactive Stores đơn giản (ví dụ: `toast.svelte.ts` để phát thông báo toàn app).

### 2.2. Xử Lý Âm Thanh Nhị Phân & Web Audio API (Client-Side Audio Engine)
Thay vì đẩy gánh nặng xử lý file về cho Backend, Frontend tự thực hiện các tác vụ âm thanh nặng ngay trên trình duyệt thông qua file `lib/audioWav.ts`:
- **Ghép nối file WAV (WAV Concatenation)**: Đọc cấu trúc header RIFF/WAV (`ArrayBuffer`, `DataView`), ghép trực tiếp dữ liệu PCM thô của nhiều chunk âm thanh thành 1 tệp WAV hoàn chỉnh duy nhất.
- **Waveform Rendering (Canvas 2D)**: Component `WaveformTrimmer.svelte` phân tích dữ liệu mảng byte âm thanh, giải mã audio buffer và vẽ dạng sóng biểu diễn trực quan trên `HTML5 Canvas` với tốc độ 60fps.
- **Cắt / Cúp Âm Thanh (Audio Trimming)**: Người dùng có thể kéo thả điểm đầu/cuối trên dạng sóng để cắt lấy đoạn audio mong muốn và xuất file nhị phân trực tiếp.

### 2.3. Nhận Dữ Liệu Phát Trực Tiếp (Audio Streaming Consumption)
- Đơn vị `StreamingPanel.svelte` sử dụng **Fetch API kết hợp `ReadableStream`** để nhận các đoạn audio byte từ backend theo thời gian thực.
- Ngay khi nhận đủ 1 chunk, âm thanh được đưa vào hàng đợi phát (Audio Queue) giúp giảm độ trễ (latency) xuống mức thấp nhất (người dùng nghe thấy tiếng nói ngay sau vài trăm milisecond).

### 2.4. Tối Ưu Giao Diện Di Động (Mobile-First Responsiveness)
- **Single-row Header Navigation**: Thanh menu điều hướng dạng cuộn ngang nhẹ nhàng trên màn hình nhỏ mà không làm đứt gãy layout.
- **Dynamic Search & Replace**: Bộ công cụ chuyển đổi linh hoạt giữa dạng ngang (Desktop) và dạng cột dọc (Mobile) đảm bảo không bị tràn khung nhìn.

---

## 3. Cấu Trúc Thư Mục Mã Nguồn Frontend

```text
frontend/
├── public/                 # Static assets (fonts, fontawesome icons đã bundle)
├── scripts/
│   └── prerender.js        # Script Node.js tự động prerender HTML tĩnh lúc build
├── src/
│   ├── assets/             # Hình ảnh, biểu trưng static
│   ├── lib/                # Thư viện core & tiện ích
│   │   ├── api.ts          # Thư viện gọi API (Fetch wrapper, error handling, session)
│   │   ├── audioWav.ts     # Xử lý nhị phân WAV header, ghép audio client-side
│   │   ├── toast.svelte.ts # Toast notification state store
│   │   ├── types.ts        # TypeScript interfaces & types định nghĩa dữ liệu
│   │   └── components/     # Các Svelte UI components
│   │       ├── AdminModal.svelte
│   │       ├── AuthModal.svelte
│   │       ├── CreateVoiceModal.svelte
│   │       ├── GenericEnginePanel.svelte
│   │       ├── Header.svelte
│   │       ├── HistoryModal.svelte
│   │       ├── StreamingPanel.svelte
│   │       ├── TextInputPanel.svelte
│   │       ├── Toast.svelte
│   │       ├── VoiceSelect.svelte
│   │       └── WaveformTrimmer.svelte
│   ├── App.svelte          # Main Application Entry Component
│   ├── app.css             # System Design Tokens, HSL Palette, CSS Core Rules
│   └── main.ts             # TypeScript Entrypoint
├── Dockerfile              # Multi-stage Docker build config
├── nginx.conf              # Nginx production reverse proxy & static serving
├── package.json            # Dependencies & Scripts
├── svelte.config.js        # Svelte compiler options
├── tsconfig.json           # TypeScript configuration
└── vite.config.ts          # Vite bundler configuration
```

---

## 4. Đóng Gói Và Triển Khai (Containerization & Deployment)

### 4.1. Quy Trình Multi-stage Docker Build
Ứng dụng được đóng gói qua 2 giai đoạn trong `Dockerfile`:
1. **Stage 1: Build Environment (`node:22-alpine`)**
   - Chạy `npm run prebuild` để copy fonts và fontawesome icons vào thư mục public.
   - Chạy `vite build` và `prerender.js` để tạo ra thư mục tĩnh `dist/`.
2. **Stage 2: Production Runtime (`nginx:alpine`)**
   - Đưa sản phẩm đã đóng gói vào webserver Nginx siêu nhẹ.

### 4.2. Cấu Hình Nginx (`nginx.conf`)
- **Port**: Lắng nghe tại cổng `5173`.
- **SPA Fallback**: `try_files $uri $uri/ /index.html;` xử lý Client-side routing.
- **Reverse Proxy**: Proxy ngược `/api/` và `/storage/` sang service `ai-backend:8000`.
- **Compression & Caching**: Bật `gzip` nén tài liệu văn bản/JS/CSS và đặt `Cache-Control` dài hạn cho static assets.

---

## 5. Lệnh Thao Tác Khi Phát Triển (Development Commands)

```bash
# 1. Chạy môi trường phát triển (Dev server HMR)
npm run dev

# 2. Kiểm tra lỗi TypeScript & Svelte Syntax
npm run check

# 3. Đóng gói sản phẩm Production (Build & Prerender)
npm run build

# 4. Xem trước bản build Production
npm run preview
```
