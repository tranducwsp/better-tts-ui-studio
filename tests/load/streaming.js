/**
 * WebSocket/SSE specialty — test task streaming under load.
 *
 *   k6 run tests/load/streaming.js
 *   k6 run -e BASE_URL=http://host:8000 tests/load/streaming.js
 *
 * Note: k6 does not support SSE natively, so this test uses polling to simulate
 * a streaming client. When k6 supports SSE, switch to event stream.
 */
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';
import {
  login, authHeaders, randomText, errorRate,
  BASE_URL, ADMIN_USER, ADMIN_PASS, USER_USER, USER_PASS, VOICES,
} from './config.js';

const streamLatency = new Trend('stream_first_event_ms', true);
const chunksPerJob  = new Trend('chunks_per_job', true);
const streamErrors  = new Counter('stream_errors');

export const options = {
  stages: [
    { duration: '2m', target: 500 },
    { duration: '5m', target: 2000 },
    { duration: '3m', target: 2000 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    stream_first_event_ms: ['p(95)<3000'],
    errors:               ['rate<0.05'],
    checks:               ['rate>0.95'],
  },
};

export default function () {
  const isAdmin = __VU % 2 === 0;
  const token = login(isAdmin ? ADMIN_USER : USER_USER, isAdmin ? ADMIN_PASS : USER_PASS);
  if (!token) { errorRate.add(1); sleep(2); return; }
  const headers = authHeaders(token);

  // Send synthesis job
  const mode = 'standard';
  const voice = VOICES[Math.floor(Math.random() * VOICES.length)];
  const res = http.post(
    `${BASE_URL}/api/synthesize/${mode}`,
    JSON.stringify({ text: randomText(), voice, speed: 1.0 }),
    { headers, tags: { name: 'synthesize' } },
  );

  if (!check(res, { 'synth ok': (r) => r.status === 200 })) {
    errorRate.add(1);
    return;
  }

  const taskId = res.json('task_id');
  const startMs = Date.now();

  // Poll task status — simulate SSE client
  let chunks = 0;
  let firstEvent = true;
  let done = false;
  const maxWaitMs = 60000;

  while (!done && Date.now() - startMs < maxWaitMs) {
    const status = http.get(`${BASE_URL}/api/tasks/${taskId}`, {
      headers,
      tags: { name: 'task_poll' },
    });

    if (status.status !== 200) {
      streamErrors.add(1);
      break;
    }

    const body = status.json();
    if (firstEvent) {
      streamLatency.add(Date.now() - startMs);
      firstEvent = false;
    }

    if (body.status === 'done') {
      done = true;
      chunksPerJob.add(body.progress ? Math.ceil(body.progress / 100) : 1);
    } else if (body.status === 'failed') {
      streamErrors.add(1);
      break;
    }

    sleep(0.3);  // Poll every 300ms — like SSE client
  }

  if (!done) {
    errorRate.add(1);
  }

  sleep(1);
}
