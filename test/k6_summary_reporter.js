export function generateHTMLReport(data) {
  const metrics = data.metrics || {};
  const httpDuration = metrics.http_req_duration ? metrics.http_req_duration.values : {};
  const httpReqs = metrics.http_reqs ? metrics.http_reqs.values.count : 0;
  const httpFailed = metrics.http_req_failed ? (metrics.http_req_failed.values.rate * 100).toFixed(2) : 0;
  const jobsDone = metrics.jobs_completed ? metrics.jobs_completed.values.count : 0;

  return `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>k6 5-Hour Test Summary</title>
  <style>
    body { font-family: sans-serif; background: #0f172a; color: #f8fafc; padding: 20px; }
    .card { background: #1e293b; padding: 20px; border-radius: 8px; margin-bottom: 20px; }
    h1 { color: #38bdf8; }
    table { width: 100%; border-collapse: collapse; margin-top: 10px; }
    th, td { padding: 10px; border: 1px solid #334155; text-align: left; }
    th { background: #334155; }
  </style>
</head>
<body>
  <h1>📊 k6 5-Hour Test Executive Summary</h1>
  <div class="card">
    <h2>Overall Metrics</h2>
    <table>
      <tr><th>Metric</th><th>Value</th></tr>
      <tr><td>Total HTTP Requests</td><td>${httpReqs.toLocaleString()}</td></tr>
      <tr><td>HTTP Failure Rate</td><td>${httpFailed}%</td></tr>
      <tr><td>Total Jobs Completed</td><td>${jobsDone}</td></tr>
      <tr><td>p(95) Latency</td><td>${(httpDuration['p(95)'] || 0).toFixed(2)} ms</td></tr>
      <tr><td>p(90) Latency</td><td>${(httpDuration['p(90)'] || 0).toFixed(2)} ms</td></tr>
      <tr><td>Average Latency</td><td>${(httpDuration['avg'] || 0).toFixed(2)} ms</td></tr>
    </table>
  </div>
</body>
</html>`;
}
