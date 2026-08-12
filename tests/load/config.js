/**
 * k6 load test — AI Voice Studio
 *
 * Run:
 *   k6 run test/smoke.js                     # 1 VU, quick
 *   k6 run test/load.js                      # 10k VU, 30-minute ramp
 *   k6 run -e BASE_URL=http://host:8000 test/load.js
 *   k6 run -e STAGE=stress test/load.js      # 10k VU, 5-minute ramp
 *   k6 cloud test/load.js                    # run on k6 Cloud
 */

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { SharedArray } from 'k6/data';

// ── Customize via environment variables ──────────────────────────────────────
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8000';
const STAGE    = __ENV.STAGE    || 'load';   // smoke | load | stress | soak

// ── Pre-created accounts ─────────────────────────────────────────────────────
const ADMIN_USER = 'admin';
const ADMIN_PASS = 'devadmin';
const USER_USER  = 'user';
const USER_PASS  = 'devuser';

// ── Custom metrics ───────────────────────────────────────────────────────────
const errorRate      = new Rate('errors');
const synthDuration  = new Trend('synthesis_e2e_ms', true);   // submit → done
const audioBytes     = new Trend('audio_response_bytes', true);

// ── Sample data ──────────────────────────────────────────────────────────────
const SAMPLE_TEXTS = new SharedArray('sample_texts', () => [
  'Xin chào! Đây là ứng dụng tổng hợp giọng nói tiếng Việt.',
  'Hôm nay thời tiết rất đẹp, trời trong xanh, không có mây.',
  'Chào mừng bạn đến với AI Voice Studio, nền tảng tổng hợp giọng nói hàng đầu Việt Nam.',
  'Việt Nam là một quốc gia đông dân, nằm ở bán đảo Đông Dương.',
  'Công nghệ trí tuệ nhân tạo đang thay đổi cách chúng ta sống và làm việc mỗi ngày.',
]);

const VOICES = ['hoai_my', 'nam_minh', 'thu_hien'];
const MODES  = ['fast', 'standard', 'zero_shot_clone'];

// ── Stages ──────────────────────────────────────────────────────────────────

const stages = {
  // Smoke — 1 VU, quick check that all endpoints work
  smoke: {
    vus: 1,
    duration: '30s',
  },

  // Load — 10k VU, gradual 30-minute ramp
  load: {
    stages: [
      { duration: '5m',  target: 1000 },   // warm-up
      { duration: '10m', target: 5000 },   // ramp
      { duration: '10m', target: 10000 },  // peak
      { duration: '5m',  target: 10000 },  // sustain
      { duration: '5m',  target: 0 },      // ramp-down
    ],
    thresholds: {
      http_req_duration:          ['p(95)<2000', 'p(99)<5000'],
      errors:                     ['rate<0.05'],
      synthesis_e2e_ms:          ['p(95)<15000', 'p(99)<30000'],
      audio_response_bytes:      ['p(99)>0'],
      checks:                     ['rate>0.95'],
    },
  },

  // Stress — 10k VU, fast 5-minute ramp
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

  // Soak — 3k VU, run 2 hours to check for leaks
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

// ── Helper ───────────────────────────────────────────────────────────────────

/** Login, returns access_token or null. */
export function login(username, password) {
  const res = http.post(
    `${BASE_URL}/api/login`,
    JSON.stringify({ username, password }),
    { headers: { 'Content-Type': 'application/json' }, tags: { name: 'login' } },
  );
  const ok = check(res, {
    'login 200': (r) => r.status === 200,
    'has token':  (r) => r.json('access_token') !== '',
  });
  if (!ok) errorRate.add(1);
  return ok ? res.json('access_token') : null;
}

/** Create Authorization header. */
export function authHeaders(token) {
  return {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${token}`,
  };
}

/** Random text from the sample list. */
export function randomText() {
  return SAMPLE_TEXTS[Math.floor(Math.random() * SAMPLE_TEXTS.length)];
}

/** Random voice. */
export function randomVoice() {
  return VOICES[Math.floor(Math.random() * VOICES.length)];
}

/** Random mode. */
export function randomMode() {
  return MODES[Math.floor(Math.random() * MODES.length)];
}

/** Poll task until done or timeout. Returns { status, elapsed_ms }. */
export function pollTask(token, taskId, maxWaitMs = 30000) {
  const start = Date.now();
  const interval = 500;
  while (Date.now() - start < maxWaitMs) {
    const res = http.get(
      `${BASE_URL}/api/tasks/${taskId}`,
      { headers: authHeaders(token), tags: { name: 'task_status' } },
    );
    if (res.status !== 200) {
      errorRate.add(1);
      return { status: 'http_error', elapsed_ms: Date.now() - start };
    }
    const body = res.json();
    if (body.status === 'done') {
      return { status: 'done', elapsed_ms: Date.now() - start };
    }
    if (body.status === 'failed') {
      errorRate.add(1);
      return { status: 'failed', elapsed_ms: Date.now() - start };
    }
    sleep(interval / 1000);
  }
  return { status: 'timeout', elapsed_ms: Date.now() - start };
}

// ── Exported for specialized scripts ────────────────────────────────────────
export { BASE_URL, STAGE, ADMIN_USER, ADMIN_PASS, USER_USER, USER_PASS, errorRate, synthDuration, audioBytes, VOICES, MODES, SAMPLE_TEXTS };