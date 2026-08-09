# Example TTS Engine (`core-tts-test`)

Bản tham khảo minh hoạ **contract** giữa Engine và nền tảng Better TTS UI Studio.

AI engineer dùng bản này để hiểu nền tảng mong đợi gì ở engine — endpoints, manifest,
format phản hồi — rồi tự thay bằng engine thật (Python, C++, Rust, CUDA…). Nền tảng
chỉ cần biết `CORE_ENGINE_URL` để kết nối; mọi thứ khác là nội bộ của engine.

---

## Contract

Nền tảng gọi đúng 5 endpoint:

| Endpoint | Method | Mục đích |
|:---|:---|:---|
| `/info` | GET | Trả manifest khai báo năng lực, giới hạn, UI schema |
| `/voices` | GET | Trả danh sách giọng preset (lọc theo `?model_id=`) |
| `/synthesize` | POST | Nhận text + voice + params, trả file âm thanh |
| `/tasks/{id}/audio` | GET | Trả file âm thanh theo format (`?format=wav\|mp3`) |
| `/voices/clone` | POST | Nhận file tham chiếu, trả voice_id mới |

### Manifest (`/info`)

Manifest là trung tâm — nền tảng đọc nó để biết:

- **Modes**: `standard`, `fast`, `clone`, … mỗi mode có thể khác capabilities
- **Capabilities**: `supports_pitch`, `supports_emotion`, `supports_cloning`, …
- **Constraints**: `max_text_length`, `speed_range`, `pitch_range`, `supported_emotions`
- **Audio spec**: `default_format` (wav/mp3), `default_sample_rate`, `reference_audio_formats`
- **UI schema**: cách trình bày controls, banner, metadata fields

Xem `schemas.py` để biết đầy đủ các trường.

### `/synthesize` request

```json
{
  "text": "Xin chào",
  "voice": "Voice A",
  "engine": "standard",
  "speed": 1.0,
  "pitch": 0.0,
  "output_format": "wav",
  "task_id": "optional-client-id"
}
```

Phản hồi: file âm thanh binary (`audio/wav` hoặc `audio/mpeg`) kèm header `X-Task-ID`.

---

## Chạy

### Docker Compose (khuyên dùng)

Trong `docker-compose.yml`, trỏ `core-engine` sang bản example:

```yaml
core-engine:
  build:
    context: ./core-tts-test
```

### Chạy trực tiếp

```bash
cd core-tts-test
pip install -r requirements.txt
python main.py
```

### Biến môi trường

| Biến | Mặc định | Mục đích |
|:---|:---|:---|
| `CORE_PORT` | `8001` | Cổng lắng |
| `CORE_HOST` | `0.0.0.0` | Host lắng |
| `MOCK_DELAY_SEC` | `0.0` | Giả lập độ trễ (chỉ dùng cho example) |

**Lưu ý**: bản example chỉ dùng `CORE_PORT` và `CORE_HOST`. Engine thật tự quyết định
biến môi trường nội bộ (ví dụ `OMP_NUM_THREADS`, `CUDA_VISIBLE_DEVICES`, …) — nền tảng
không áp đặt hay đọc bất kỳ biến nào ngoài `CORE_ENGINE_URL`.

---

## Âm thanh

Bản example sinh sóng sin ngẫu nhiên dài **5–10 giây**, 24kHz, PCM 16-bit — không cần
GPU hay thư viện ML. Engine thật thay bằng model inference.

---

## Thay engine thật

1. Viết service expose cùng 5 endpoint trên
2. Trả manifest `/info` đúng schema (xem `schemas.py`)
3. Đổi `CORE_ENGINE_URL` trong compose trỏ sang engine mới
4. Xong — nền tảng không cần thay gì
