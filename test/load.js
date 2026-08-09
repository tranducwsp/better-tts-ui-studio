/**
 * Load test — 10k người dùng đồng thời.
 *
 * Mỗi VU mô phỏng 1 người dùng thực: login → duyệt → tổng hợp → nghe → lặp lại.
 *
 *   k6 run test/load.js                                # 10k VU, ramp 30 phút
 *   k6 run -e STAGE=stress test/load.js                # 10k VU, ramp 5 phút
 *   k6 run -e STAGE=soak test/load.js                  # 3k VU, 2 giờ
 *   k6 run -e BASE_URL=http://host:8000 test/load.js
 *   k6 run -e STAGE=smoke test/load.js                 # 1 VU, nhanh
 */
import http from 'k6/http';
import { check, sleep, group } from 'k6';
import {
  login, authHeaders, randomText, randomVoice, randomMode,
  pollTask, errorRate, synthDuration, audioBytes,
  BASE_URL, STAGE, ADMIN_USER, ADMIN_PASS, USER_USER, USER_PASS,
  VOICES, MODES, SAMPLE_TEXTS,
} from './config.js';

// ── Stages & thresholds (import từ config) ──────────────────────────────────
// config.js đã export `options` qua stage, nhưng k6 cần options ở top-level.
// Nên ta định nghĩa lại ở đây.

const stages = {
  smoke: { vus: 1, duration: '30s' },
  load: {
    stages: [
      { duration: '5m',  target: 1000 },
      { duration: '10m', target: 5000 },
      { duration: '10m', target: 10000 },
      { duration: '5m',  target: 10000 },
      { duration: '5m',  target: 0 },
    ],
    thresholds: {
      http_req_duration:          ['p(95)<2000', 'p(99)<5000'],
      errors:                     ['rate<0.05'],
      synthesis_e2e_ms:          ['p(95)<15000', 'p(99)<30000'],
      audio_response_bytes:      ['p(99)>0'],
      checks:                     ['rate>0.95'],
    },
  },
  stress: {
    stages: [
      { duration: '1m',  target: 5000 },
      { duration: '2m',  target: 10000 },
      { duration: '3m',  target: 10000 },
      { duration: '1m',  target: 0 },
    ],
    thresholds: {
      http_req_duration:          ['p(95)<5000', 'p(99)<10000'],
      errors:                     ['rate<0.10'],
      synthesis_e2e_ms:          ['p(95)<30000'],
      checks:                     ['rate>0.90'],
    },
  },
  soak: {
    stages: [
      { duration: '5m',  target: 3000 },
      { duration: '110m', target: 3000 },
      { duration: '5m',  target: 0 },
    ],
    thresholds: {
      http_req_duration:          ['p(95)<2000'],
      errors:                     ['rate<0.02'],
      checks:                     ['rate>0.97'],
    },
  },
};

const cfg = stages[STAGE];

export const options = STAGE === 'smoke'
  ? { vus: 1, duration: '30s', thresholds: { checks: ['rate>0.99'], errors: ['rate<0.01'] } }
  : { stages: cfg.stages, thresholds: cfg.thresholds };

// ── VU setup: login 1 lần trước khi lặp ─────────────────────────────────────
let token = null;

export function setup() {
  // Verify hệ thống lên trước khi chạy
  const health = http.get(`${BASE_URL}/health`);
  if (health.status !== 200) {
    throw new Error(`Backend không sẵn sàng: ${health.status}`);
  }
  const info = http.get(`${BASE_URL}/api/info`);
  if (info.status !== 200) {
    throw new Error(`Manifest không tải được: ${info.status}`);
  }
  console.log(`✅ Backend OK. Modes: ${info.json('supported_modes')?.map(m => m.id).join(', ')}`);
}

export default function () {
  // ── Login (1 lần mỗi VU iteration) ────────────────────────────────────
  // Dùng 2 tài khoản tạo sẵn, luân phiên theo VU id để tránh rate limit
  const isAdmin = __VU % 2 === 0;
  const username = isAdmin ? ADMIN_USER : USER_USER;
  const password = isAdmin ? ADMIN_PASS : USER_PASS;

  token = login(username, password);
  if (!token) {
    errorRate.add(1);
    sleep(2);
    return;
  }
  const headers = authHeaders(token);

  // ── User journey: duyệt → tổng hợp → nghe → lặp ─────────────────────

  // 1. Mở trang → fetch manifest
  group('browse', () => {
    const info = http.get(`${BASE_URL}/api/info`, { tags: { name: 'info' } });
    check(info, { 'info ok': (r) => r.status === 200 });
  });

  // 2. Chọn mode → fetch voices
  const mode = randomMode();
  group('voices', () => {
    const voices = http.get(`${BASE_URL}/api/voices/${mode}`, { headers, tags: { name: 'voices' } });
    check(voices, { 'voices ok': (r) => r.status === 200 });
  });

  // 3. Nhập text → tổng hợp
  const voice = mode === 'fast' ? VOICES[Math.floor(Math.random() * 2)] : randomVoice();
  const text  = randomText();

  group('synthesize', () => {
    const body = { text, voice, speed: 1.0 };
    if (mode === 'zero_shot_clone') {
      // Clone mode — thêm voice_id thay vì voice
      body.voice = voice;
    }
    const res = http.post(
      `${BASE_URL}/api/synthesize/${mode}`,
      JSON.stringify(body),
      { headers, tags: { name: 'synthesize' } },
    );

    const ok = check(res, {
      'synth accepted': (r) => r.status === 200 || r.status === 202,
      'has task_id':    (r) => r.json('task_id') !== '',
    });

    if (!ok) {
      errorRate.add(1);
      return;
    }

    const taskId = res.json('task_id');

    // 4. Đợi kết quả (poll)
    const result = pollTask(token, taskId, 60000);
    synthDuration.add(result.elapsed_ms);

    if (result.status === 'done') {
      // 5. Tải audio
      const audio = http.get(`${BASE_URL}/api/tasks/${taskId}/audio`, {
        headers,
        tags: { name: 'audio' },
      });
      check(audio, { 'audio ok': (r) => r.status === 200 });
      audioBytes.add(audio.body.length);
    } else {
      errorRate.add(1);
    }
  });

  // 6. Xem lịch sử (30% người dùng)
  if (Math.random() < 0.3) {
    group('history', () => {
      const hist = http.get(`${BASE_URL}/api/history`, { headers, tags: { name: 'history' } });
      check(hist, { 'history ok': (r) => r.status === 200 });
    });
  }

  // Nghỉ ngẫu nhiên 1–5 giây — mô phỏng người dùng thực
  sleep(1 + Math.random() * 4);
}

export function teardown(data) {
  // Cleanup: không cần, backend tự quản lý
}
