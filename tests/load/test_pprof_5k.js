import http from 'k6/http';
import { check, sleep } from 'k6';
import {
  login, authHeaders, randomText, randomVoice, randomMode,
  pollTask, errorRate, synthDuration, audioBytes,
  BASE_URL, ADMIN_USER, ADMIN_PASS, USER_USER, USER_PASS,
  VOICES, MODES,
} from './config.js';

export const options = {
  stages: [
    { duration: '30s', target: 5000 },
    { duration: '1m',  target: 5000 },
    { duration: '30s', target: 0 },
  ],
};

export function setup() {
  const adminToken = login(ADMIN_USER, ADMIN_PASS);
  const userToken = login(USER_USER, USER_PASS);
  return { adminToken, userToken };
}

export default function (data) {
  const token = data.userToken;
  if (!token) return;
  const headers = authHeaders(token);
  const mode = 'fast';
  const voice = 'nam_minh';

  const res = http.post(
    `${BASE_URL}/api/synthesize/${mode}`,
    JSON.stringify({
      text: randomText(),
      voice_id: voice,
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
