import http from 'k6/http';
import { check, sleep } from 'k6';
import {
  login, authHeaders, randomText,
  pollTask,
  BASE_URL, ADMIN_USER, ADMIN_PASS, USER_USER, USER_PASS
} from './config.js';

export const options = {
  stages: [
    { duration: '30s', target: 0 },    // 30s rest (baseline RAM)
    { duration: '15s', target: 5000 }, // fast ramp up to 5k
    { duration: '1m',  target: 5000 }, // 1m sustain 5k VUs
    { duration: '15s', target: 0 },    // ramp down
    { duration: '1m',  target: 0 },    // 1m final rest (verify RAM baseline recovery)
  ],
};

export function setup() {
  const userToken = login(USER_USER, USER_PASS);
  return { userToken };
}

export default function (data) {
  const token = data.userToken;
  if (!token) return;
  const headers = authHeaders(token);

  const res = http.post(
    `${BASE_URL}/api/synthesize/fast`,
    JSON.stringify({
      text: randomText(),
      voice_id: 'nam_minh',
      speed: 1.0,
      format: 'mp3',
    }),
    headers
  );

  if (res.status === 200 || res.status === 202) {
    try {
      const task = JSON.parse(res.body);
      if (task.id) {
        pollTask(task.id, headers);
      }
    } catch (e) {}
  }
  sleep(0.5);
}
