/**
 * 1-Hour Fixed 5000 VUs Endurance Test
 * 2 Waves (30 minutes each): 10 min load (5000 VUs) + 20 min rest (0 VUs)
 */
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';
import { generateHTMLReport } from './k6_summary_reporter.js';
import {
  login, authHeaders, randomText, randomVoice, randomMode,
  pollTask, errorRate, synthDuration, audioBytes,
  BASE_URL, ADMIN_USER, ADMIN_PASS, USER_USER, USER_PASS,
  VOICES, MODES,
} from './config.js';

const jobsCompleted = new Counter('jobs_completed');
const jobsFailed = new Counter('jobs_failed');
const audioDownloadMs = new Trend('audio_download_ms', true);

export const options = {
  stages: [
    // Wave 1: Fixed 5000 VUs
    { duration: '1m', target: 5000 },
    { duration: '8m', target: 5000 },
    { duration: '1m', target: 0 },
    { duration: '20m', target: 0 },

    // Wave 2: Fixed 5000 VUs
    { duration: '1m', target: 5000 },
    { duration: '8m', target: 5000 },
    { duration: '1m', target: 0 },
    { duration: '20m', target: 0 },
  ],
  thresholds: {
    errors: ['rate<0.10'],
    checks: ['rate>0.90'],
  },
};

export function setup() {
  const adminToken = login(ADMIN_USER, ADMIN_PASS);
  const userToken = login(USER_USER, USER_PASS);
  return { adminToken, userToken };
}

export default function (data) {
  const isAdmin = __VU % 2 === 0;
  const token = isAdmin ? data.adminToken : data.userToken;
  if (!token) { errorRate.add(1); sleep(2); return; }
  const headers = authHeaders(token);

  const mode = randomMode();
  const voice = mode === 'fast'
    ? VOICES[Math.floor(Math.random() * 2)]
    : randomVoice();

  const res = http.post(
    `${BASE_URL}/api/synthesize/${mode}`,
    JSON.stringify({ text: randomText(), voice, speed: 1.0 }),
    { headers, tags: { name: 'synthesize', mode } },
  );

  const ok = check(res, {
    'synth accepted': (r) => r.status === 200 || r.status === 202,
    'has task_id': (r) => r.json('task_id') !== '',
  });

  if (!ok) {
    errorRate.add(1);
    jobsFailed.add(1);
    sleep(1);
    return;
  }

  const taskId = res.json('task_id');
  const result = pollTask(token, taskId, 60000);
  synthDuration.add(result.elapsed_ms);

  if (result.status === 'done') {
    jobsCompleted.add(1);
    const dlStart = Date.now();
    const audio = http.get(`${BASE_URL}/api/tasks/${taskId}/audio`, {
      headers,
      tags: { name: 'audio' },
    });
    audioDownloadMs.add(Date.now() - dlStart);
    check(audio, { 'audio ok': (r) => r.status === 200 });
    audioBytes.add(audio.body ? audio.body.length : 0);
  } else {
    jobsFailed.add(1);
    errorRate.add(1);
  }
}

export function handleSummary(data) {
  return {
    '/test/summary_1h.html': generateHTMLReport(data),
  };
}
