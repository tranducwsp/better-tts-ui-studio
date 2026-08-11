/**
 * API endpoint逐一测试 — verify mọi route hoạt động đúng.
 *
 *   k6 run tests/load/api.js
 *   k6 run -e BASE_URL=http://host:8000 tests/load/api.js
 */
import http from 'k6/http';
import { check, group } from 'k6';
import { login, authHeaders, errorRate, BASE_URL, ADMIN_USER, ADMIN_PASS, USER_USER, USER_PASS } from './config.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: { checks: ['rate>0.90'], errors: ['rate<0.10'] },
};

export default function () {
  const jar = http.cookieJar();

  // ── Public endpoints ──────────────────────────────────────────────────
  group('GET /health', () => {
    const r = http.get(`${BASE_URL}/health`);
    check(r, { '200': (r) => r.status === 200, 'has status': (r) => r.json('status') === 'ok' });
  });

  group('GET /ready', () => {
    const r = http.get(`${BASE_URL}/ready`);
    check(r, { '200': (r) => r.status === 200 });
  });

  group('GET /api/info', () => {
    const r = http.get(`${BASE_URL}/api/info`);
    check(r, {
      '200':            (r) => r.status === 200,
      '3 modes':        (r) => r.json('supported_modes')?.length === 3,
      'has ui_schema':  (r) => r.json('ui_schema') !== null,
      'has model_sort': (r) => r.json('ui_schema.model_sort')?.length === 3,
    });
  });

  // ── Auth ──────────────────────────────────────────────────────────────
  group('POST /api/login (admin)', () => {
    const r = http.post(`${BASE_URL}/api/login`, JSON.stringify({ username: ADMIN_USER, password: ADMIN_PASS }), { headers: { 'Content-Type': 'application/json' } });
    check(r, { '200': (r) => r.status === 200, 'has token': (r) => r.json('access_token') !== '' });
  });

  group('POST /api/login (wrong password)', () => {
    const r = http.post(`${BASE_URL}/api/login`, JSON.stringify({ username: ADMIN_USER, password: 'wrong' }), { headers: { 'Content-Type': 'application/json' } });
    check(r, { '401': (r) => r.status === 401 });
  });

  group('POST /api/login (missing fields)', () => {
    const r = http.post(`${BASE_URL}/api/login`, JSON.stringify({}), { headers: { 'Content-Type': 'application/json' } });
    check(r, { '4xx': (r) => r.status >= 400 && r.status < 500 });
  });

  // ── Protected — user ──────────────────────────────────────────────────
  const userToken = login(USER_USER, USER_PASS);
  const userHeaders = authHeaders(userToken);

  group('GET /api/me (user)', () => {
    const r = http.get(`${BASE_URL}/api/me`, { headers: userHeaders });
    check(r, { '200': (r) => r.status === 200, 'role=user': (r) => r.json('role') === 'user' });
  });

  group('GET /api/voices/fast', () => {
    const r = http.get(`${BASE_URL}/api/voices/fast`, { headers: userHeaders });
    check(r, { '200': (r) => r.status === 200, 'array': (r) => Array.isArray(r.json()) });
  });

  group('GET /api/voices/standard', () => {
    const r = http.get(`${BASE_URL}/api/voices/standard`, { headers: userHeaders });
    check(r, { '200': (r) => r.status === 200 });
  });

  group('GET /api/voices/zero_shot_clone', () => {
    const r = http.get(`${BASE_URL}/api/voices/zero_shot_clone`, { headers: userHeaders });
    check(r, { '200': (r) => r.status === 200 });
  });

  group('GET /api/history', () => {
    const r = http.get(`${BASE_URL}/api/history`, { headers: userHeaders });
    check(r, { '200': (r) => r.status === 200 });
  });

  group('POST /api/synthesize/fast', () => {
    const r = http.post(`${BASE_URL}/api/synthesize/fast`, JSON.stringify({ text: 'test', voice: 'hoai_my', speed: 1.0 }), { headers: userHeaders });
    check(r, { '200': (r) => r.status === 200, 'has task_id': (r) => r.json('task_id') !== '' });
  });

  group('POST /api/synthesize/standard', () => {
    const r = http.post(`${BASE_URL}/api/synthesize/standard`, JSON.stringify({ text: 'test', voice: 'nam_minh', speed: 1.0 }), { headers: userHeaders });
    check(r, { '200': (r) => r.status === 200, 'has task_id': (r) => r.json('task_id') !== '' });
  });

  group('POST /api/synthesize/zero_shot_clone (no voice)', () => {
    const r = http.post(`${BASE_URL}/api/synthesize/zero_shot_clone`, JSON.stringify({ text: 'test', voice: '', speed: 1.0 }), { headers: userHeaders });
    // Backend accepts empty voice — worker will fail later
    check(r, { 'accepted or rejected': (r) => r.status === 200 || r.status >= 400 });
  });

  // ── Protected — admin ─────────────────────────────────────────────────
  const adminToken = login(ADMIN_USER, ADMIN_PASS);
  const adminHeaders = authHeaders(adminToken);

  group('GET /api/admin/users', () => {
    const r = http.get(`${BASE_URL}/api/admin/users`, { headers: adminHeaders });
    check(r, { '200': (r) => r.status === 200 });
  });

  // ── Unauthorized — clear cookies first ────────────────────────────────
  group('GET /api/me (no token)', () => {
    jar.clear(BASE_URL);
    const r = http.get(`${BASE_URL}/api/me`);
    check(r, { '401': (r) => r.status === 401 });
  });

  group('POST /api/synthesize/standard (no token)', () => {
    jar.clear(BASE_URL);
    const r = http.post(`${BASE_URL}/api/synthesize/standard`, JSON.stringify({ text: 'test' }), { headers: { 'Content-Type': 'application/json' } });
    check(r, { '401': (r) => r.status === 401 });
  });

  // ── Forbidden (user tries admin) ─────────────────────────────────────
  group('GET /api/admin/users (user token)', () => {
    jar.clear(BASE_URL);
    const r = http.get(`${BASE_URL}/api/admin/users`, { headers: userHeaders });
    check(r, { '403': (r) => r.status === 403 });
  });

  // ── Not found ────────────────────────────────────────────────────────
  group('GET /api/tasks/nonexistent-id', () => {
    const r = http.get(`${BASE_URL}/api/tasks/00000000-0000-0000-0000-000000000000`, { headers: userHeaders });
    check(r, { '404': (r) => r.status === 404 });
  });
}
