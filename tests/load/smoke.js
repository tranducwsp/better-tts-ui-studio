/**
 * Smoke test — 1 VU, quickly runs all main endpoints.
 *
 *   k6 run tests/load/smoke.js
 *   k6 run -e BASE_URL=http://host:8000 tests/load/smoke.js
 */
import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { authHeaders, randomText, pollTask, errorRate, synthDuration, audioBytes, BASE_URL, ADMIN_USER, ADMIN_PASS } from './config.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate>0.99'],
    errors: ['rate<0.01'],
  },
};

export function setup() {
  // Login once — not subject to rate limits
  const res = http.post(
    `${BASE_URL}/api/login`,
    JSON.stringify({ username: ADMIN_USER, password: ADMIN_PASS }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  if (res.status !== 200) throw new Error(`Login failed: ${res.status}`);
  return { token: res.json('access_token') };
}

export default function (data) {
  const headers = authHeaders(data.token);

  // ── 1. Health check ──────────────────────────────────────────────────
  group('health', () => {
    const res = http.get(`${BASE_URL}/health`);
    check(res, { 'health 200': (r) => r.status === 200 });
  });

  // ── 2. Manifest ──────────────────────────────────────────────────────
  group('manifest', () => {
    const res = http.get(`${BASE_URL}/api/info`);
    check(res, {
      'info 200':       (r) => r.status === 200,
      'has 3 modes':    (r) => r.json('supported_modes')?.length === 3,
      'has fast mode':  (r) => r.json('supported_modes')?.some(m => m.id === 'fast'),
      'has model_sort': (r) => r.json('ui_schema.model_sort')?.length === 3,
    });
  });

  // ── 3. /me ───────────────────────────────────────────────────────────
  group('me', () => {
    const res = http.get(`${BASE_URL}/api/me`, { headers, tags: { name: 'me' } });
    check(res, {
      'me 200':        (r) => r.status === 200,
      'me has role':   (r) => r.json('role') !== '',
    });
  });

  // ── 4. Voices ────────────────────────────────────────────────────────
  group('voices', () => {
    for (const mode of ['fast', 'standard', 'zero_shot_clone']) {
      const res = http.get(`${BASE_URL}/api/voices/${mode}`, { headers, tags: { name: 'voices' } });
      check(res, {
        [`voices/${mode} 200`]: (r) => r.status === 200,
        [`voices/${mode} list`]: (r) => Array.isArray(r.json()),
      });
    }
  });

  // ── 5. Synthesize (fast) ─────────────────────────────────────────────
  group('synthesize_fast', () => {
    const res = http.post(
      `${BASE_URL}/api/synthesize/fast`,
      JSON.stringify({ text: randomText(), voice: 'hoai_my', speed: 1.0 }),
      { headers, tags: { name: 'synthesize' } },
    );
    const ok = check(res, {
      'synthesize 200': (r) => r.status === 200,
      'has task_id':    (r) => r.json('task_id') !== '',
    });
    if (!ok) { errorRate.add(1); return; }

    const taskId = res.json('task_id');
    const result = pollTask(data.token, taskId);
    synthDuration.add(result.elapsed_ms);
    check(result, { 'synth done': (r) => r.status === 'done' });
  });

  // ── 6. Synthesize (standard) ─────────────────────────────────────────
  group('synthesize_standard', () => {
    const res = http.post(
      `${BASE_URL}/api/synthesize/standard`,
      JSON.stringify({ text: randomText(), voice: 'nam_minh', speed: 1.0 }),
      { headers, tags: { name: 'synthesize' } },
    );
    const ok = check(res, {
      'synthesize 200': (r) => r.status === 200,
      'has task_id':    (r) => r.json('task_id') !== '',
    });
    if (!ok) { errorRate.add(1); return; }

    const taskId = res.json('task_id');
    const result = pollTask(data.token, taskId);
    synthDuration.add(result.elapsed_ms);

    // Audio download
    if (result.status === 'done') {
      const audio = http.get(`${BASE_URL}/api/tasks/${taskId}/audio`, { headers, tags: { name: 'audio' } });
      check(audio, { 'audio 200': (r) => r.status === 200 });
      audioBytes.add(audio.body.length);
    }
  });

  // ── 7. History ───────────────────────────────────────────────────────
  group('history', () => {
    const res = http.get(`${BASE_URL}/api/history`, { headers, tags: { name: 'history' } });
    check(res, { 'history 200': (r) => r.status === 200 });
  });
}
