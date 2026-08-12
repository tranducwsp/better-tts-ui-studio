/**
 * Synthesize specialty — test the synthesis endpoint under high load.
 *
 * Focus on throughput: how many jobs/minute the system can handle at 10k VU.
 *
 *   k6 run tests/load/synth.js
 *   k6 run -e BASE_URL=http://host:8000 tests/load/synth.js
 */
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';
import {
  login, authHeaders, randomText, randomVoice, randomMode,
  pollTask, errorRate, synthDuration, audioBytes,
  BASE_URL, ADMIN_USER, ADMIN_PASS, USER_USER, USER_PASS,
  VOICES, MODES,
} from './config.js';

const jobsCompleted  = new Counter('jobs_completed');
const jobsFailed     = new Counter('jobs_failed');
const audioDownloadMs = new Trend('audio_download_ms', true);

export const options = {
  stages: [
    { duration: '3m',  target: 1000 },
    { duration: '5m',  target: 5000 },
    { duration: '10m', target: 10000 },
    { duration: '5m',  target: 10000 },
    { duration: '3m',  target: 0 },
  ],
  thresholds: {
    http_req_duration:   ['p(95)<3000'],
    synthesis_e2e_ms:   ['p(95)<15000', 'p(99)<30000'],
    errors:             ['rate<0.05'],
    jobs_completed:     ['count > 0'],
    checks:             ['rate>0.95'],
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

  // Select mode & voice
  const mode = randomMode();
  const voice = mode === 'fast'
    ? VOICES[Math.floor(Math.random() * 2)]   // fast only has hoai_my, nam_minh
    : randomVoice();

  // Send synthesis request
  const res = http.post(
    `${BASE_URL}/api/synthesize/${mode}`,
    JSON.stringify({ text: randomText(), voice, speed: 1.0 }),
    { headers, tags: { name: 'synthesize', mode } },
  );

  const ok = check(res, {
    'synth accepted': (r) => r.status === 200 || r.status === 202,
    'has task_id':    (r) => r.json('task_id') !== '',
  });

  if (!ok) {
    errorRate.add(1);
    jobsFailed.add(1);
    sleep(1);
    return;
  }

  const taskId = res.json('task_id');

  // Poll until done
  const result = pollTask(token, taskId, 60000);
  synthDuration.add(result.elapsed_ms);

  if (result.status === 'done') {
    jobsCompleted.add(1);

    // Download audio
    const dlStart = Date.now();
    const audio = http.get(`${BASE_URL}/api/tasks/${taskId}/audio`, {
      headers,
      tags: { name: 'audio' },
    });
    audioDownloadMs.add(Date.now() - dlStart);
    check(audio, { 'audio ok': (r) => r.status === 200 });
    audioBytes.add(audio.body.length);
  } else {
    jobsFailed.add(1);
    errorRate.add(1);
  }

  // Short pause — user synthesizing continuously
  sleep(0.5 + Math.random() * 2);
}
