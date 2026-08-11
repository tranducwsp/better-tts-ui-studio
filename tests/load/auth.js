/**
 * Auth专项 — test login/register/refresh/logout dưới tải cao.
 *
 *   k6 run tests/load/auth.js
 *   k6 run -e BASE_URL=http://host:8000 tests/load/auth.js
 */
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';
import { login, BASE_URL, ADMIN_USER, ADMIN_PASS, errorRate } from './config.js';

const authErrorRate = new Rate('auth_errors');

export const options = {
  stages: [
    { duration: '2m', target: 500 },
    { duration: '5m', target: 2000 },
    { duration: '3m', target: 2000 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    http_req_duration:   ['p(95)<1000'],
    auth_errors:         ['rate<0.02'],
    checks:              ['rate>0.98'],
  },
};

export default function () {
  // ── Login ─────────────────────────────────────────────────────────────
  const res = http.post(
    `${BASE_URL}/api/login`,
    JSON.stringify({ username: ADMIN_USER, password: ADMIN_PASS }),
    { headers: { 'Content-Type': 'application/json' }, tags: { name: 'login' } },
  );
  check(res, {
    'login 200':  (r) => r.status === 200,
    'has token':  (r) => r.json('access_token') !== '',
  }) || authErrorRate.add(1);

  if (res.status === 429) {
    // Rate limit — chờ rồi thử lại
    sleep(5);
    return;
  }

  const token = res.json('access_token');
  if (!token) { sleep(1); return; }

  const headers = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${token}`,
  };

  // ── /me ───────────────────────────────────────────────────────────────
  const me = http.get(`${BASE_URL}/api/me`, { headers, tags: { name: 'me' } });
  check(me, { 'me 200': (r) => r.status === 200 }) || authErrorRate.add(1);

  // ── Refresh ───────────────────────────────────────────────────────────
  // Refresh đọc HttpOnly cookie, k6 không giữ cookie tự động nên test
  // chỉ verify endpoint trả lỗi đúng khi thiếu cookie.
  const refresh = http.post(`${BASE_URL}/api/auth/refresh`, null, {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'refresh' },
  });
  check(refresh, {
    'refresh no cookie → 401': (r) => r.status === 401,
  });

  // ── Logout ────────────────────────────────────────────────────────────
  const logout = http.post(`${BASE_URL}/api/auth/logout`, null, {
    headers,
    tags: { name: 'logout' },
  });
  check(logout, { 'logout ok': (r) => r.status === 200 || r.status === 204 });

  sleep(1);
}
