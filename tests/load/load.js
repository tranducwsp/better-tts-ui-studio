/**
 * Load test — 10k concurrent users.
 *
 * Each VU simulates a real user: login → browse → synthesize → listen → repeat.
 *
 *   k6 run tests/load/load.js                                # 10k VU, ramp 30 min
 *   k6 run -e STAGE=stress tests/load/load.js                # 10k VU, ramp 5 min
 *   k6 run -e STAGE=soak tests/load/load.js                  # 3k VU, 2 hours
 *   k6 run -e BASE_URL=http://host:8000 tests/load/load.js
 *   k6 run -e STAGE=smoke tests/load/load.js                 # 1 VU, nhanh
 */
import http from 'k6/http';
import { check, sleep, group } from 'k6';
import {
  login, authHeaders, randomText, randomVoice, randomMode,
  pollTask, errorRate, synthDuration, audioBytes,
  BASE_URL, STAGE, ADMIN_USER, ADMIN_PASS, USER_USER, USER_PASS,
  VOICES, MODES, SAMPLE_TEXTS,
} from './config.js';

// ── Stages & thresholds (imported from config) ──────────────────────────────────
// config.js exports `options` via stage, but k6 expects options at the top-level.
// So we define them here.

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

// ── VU setup: login once before looping ─────────────────────────────────────
let token = null;

export function setup() {
  // Verify the system is up before running
  const health = http.get(`${BASE_URL}/health`);
  if (health.status !== 200) {
    throw new Error(`Backend not ready: ${health.status}`);
  }
  const info = http.get(`${BASE_URL}/api/info`);
  if (info.status !== 200) {
    throw new Error(`Manifest could not be loaded: ${info.status}`);
  }
  console.log(`✅ Backend OK. Modes: ${info.json('supported_modes')?.map(m => m.id).join(', ')}`);
}

export default function () {
  // ── Login (once per VU iteration) ────────────────────────────────────
  // Use 2 pre-created accounts, alternating by VU id to avoid rate limits
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

  // ── User journey: browse → synthesize → listen → repeat ─────────────────────

  // 1. Open page → fetch manifest
  group('browse', () => {
    const info = http.get(`${BASE_URL}/api/info`, { tags: { name: 'info' } });
    check(info, { 'info ok': (r) => r.status === 200 });
  });

  // 2. Select mode → fetch voices
  const mode = randomMode();
  group('voices', () => {
    const voices = http.get(`${BASE_URL}/api/voices/${mode}`, { headers, tags: { name: 'voices' } });
    check(voices, { 'voices ok': (r) => r.status === 200 });
  });

  // 3. Input text → synthesize
  const voice = mode === 'fast' ? VOICES[Math.floor(Math.random() * 2)] : randomVoice();
  const text  = randomText();

  group('synthesize', () => {
    const body = { text, voice, speed: 1.0 };
    if (mode === 'zero_shot_clone') {
      // Clone mode — use voice_id instead of voice name
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

    // 4. Wait for result (poll)
    const result = pollTask(token, taskId, 60000);
    synthDuration.add(result.elapsed_ms);

    if (result.status === 'done') {
      // 5. Download audio
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

  // 6. View history (30% of users)
  if (Math.random() < 0.3) {
    group('history', () => {
      const hist = http.get(`${BASE_URL}/api/history`, { headers, tags: { name: 'history' } });
      check(hist, { 'history ok': (r) => r.status === 200 });
    });
  }

  // Random pause 1–5 seconds — simulating real user behavior
  sleep(1 + Math.random() * 4);
}

export function teardown(data) {
  // Cleanup: not needed, backend manages itself
}
