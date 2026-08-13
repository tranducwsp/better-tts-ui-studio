#!/usr/bin/env python3
import json
import subprocess
import time
import os
from datetime import datetime

METRICS_FILE = os.path.join(os.path.dirname(__file__), "..", "reports", "metrics_1h.json")
HTML_FILE = os.path.join(os.path.dirname(__file__), "..", "reports", "ram_report_1h.html")
DURATION_SECONDS = 3720  # 62 minutes
INTERVAL_SECONDS = 15

def get_docker_stats():
    try:
        cmd = ["docker", "stats", "--no-stream", "--format", "{{.Name}}|{{.MemUsage}}|{{.CPUPerc}}", "ai-backend", "ai-worker", "ai-engine"]
        res = subprocess.run(cmd, capture_output=True, text=True, check=True)
        lines = res.stdout.strip().split("\n")
        stats = {}
        for line in lines:
            if "|" in line:
                parts = line.split("|")
                name = parts[0]
                mem_raw = parts[1].split("/")[0].strip()
                cpu_raw = parts[2].strip().replace("%", "")
                
                mem_val = 0.0
                if "GiB" in mem_raw:
                    mem_val = float(mem_raw.replace("GiB", "").strip()) * 1024.0
                elif "MiB" in mem_raw:
                    mem_val = float(mem_raw.replace("MiB", "").strip())
                elif "KiB" in mem_raw or "kB" in mem_raw:
                    mem_val = float(mem_raw.replace("KiB", "").replace("kB", "").strip()) / 1024.0
                elif "B" in mem_raw:
                    mem_val = float(mem_raw.replace("B", "").strip()) / (1024.0 * 1024.0)
                
                cpu_val = float(cpu_raw) if cpu_raw else 0.0
                stats[name] = {"mem_mib": round(mem_val, 2), "cpu_pct": round(cpu_val, 2)}
        return stats
    except Exception as e:
        return {}

def render_html_report(data):
    labels = [item["time"] for item in data]
    backend_mem = [item["stats"].get("ai-backend", {}).get("mem_mib", 0) for item in data]
    worker_mem = [item["stats"].get("ai-worker", {}).get("mem_mib", 0) for item in data]
    engine_mem = [item["stats"].get("ai-engine", {}).get("mem_mib", 0) for item in data]
    backend_cpu = [item["stats"].get("ai-backend", {}).get("cpu_pct", 0) for item in data]

    html_content = f"""<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>1-Hour Fixed 5000 VUs Endurance Report</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        body {{ font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background-color: #0f172a; color: #f8fafc; margin: 0; padding: 20px; }}
        .header {{ text-align: center; padding: 20px; background: #1e293b; border-radius: 12px; margin-bottom: 20px; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.3); }}
        .header h1 {{ margin: 0; color: #38bdf8; font-size: 28px; }}
        .header p {{ color: #94a3b8; margin: 8px 0 0 0; }}
        .grid {{ display: grid; grid-template-columns: 1fr; gap: 20px; max-width: 1400px; margin: 0 auto; }}
        .card {{ background: #1e293b; border-radius: 12px; padding: 24px; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.3); }}
        .card h2 {{ margin-top: 0; color: #cbd5e1; font-size: 20px; border-bottom: 1px solid #334155; padding-bottom: 10px; }}
        .chart-container {{ position: relative; height: 400px; width: 100%; }}
        .summary-boxes {{ display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 15px; margin-bottom: 20px; }}
        .stat-box {{ background: #0f172a; border: 1px solid #334155; border-radius: 8px; padding: 16px; text-align: center; }}
        .stat-box .title {{ color: #94a3b8; font-size: 14px; margin-bottom: 6px; }}
        .stat-box .value {{ font-size: 24px; font-weight: bold; color: #38bdf8; }}
    </style>
</head>
<body>
    <div class="header">
        <h1>📊 1-Hour Fixed 5000 VUs Endurance Report</h1>
        <p>Waves: 2 Waves (5000 VUs constant) | Sampling Rate: 15s | Map Bucket Re-allocation Enabled</p>
    </div>

    <div class="summary-boxes">
        <div class="stat-box">
            <div class="title">Max Backend RAM</div>
            <div class="value">{max(backend_mem) if backend_mem else 0:.1f} MiB</div>
        </div>
        <div class="stat-box">
            <div class="title">Current Backend RAM</div>
            <div class="value">{backend_mem[-1] if backend_mem else 0:.1f} MiB</div>
        </div>
        <div class="stat-box">
            <div class="title">Worker Memory</div>
            <div class="value">{worker_mem[-1] if worker_mem else 0:.1f} MiB</div>
        </div>
        <div class="stat-box">
            <div class="title">Total Data Points</div>
            <div class="value">{len(data)}</div>
        </div>
    </div>

    <div class="grid">
        <div class="card">
            <h2>🧠 Container Memory Usage (MiB) Over 1 Hour</h2>
            <div class="chart-container">
                <canvas id="memChart"></canvas>
            </div>
        </div>
        <div class="card">
            <h2>⚡ Backend CPU Load (%) Over 1 Hour</h2>
            <div class="chart-container">
                <canvas id="cpuChart"></canvas>
            </div>
        </div>
    </div>

    <script>
        const labels = {json.dumps(labels)};
        
        new Chart(document.getElementById('memChart'), {{
            type: 'line',
            data: {{
                labels: labels,
                datasets: [
                    {{ label: 'ai-backend (Go API)', data: {json.dumps(backend_mem)}, borderColor: '#38bdf8', backgroundColor: 'rgba(56, 189, 248, 0.1)', fill: true, tension: 0.2, pointRadius: 0 }},
                    {{ label: 'ai-worker (Go Queue)', data: {json.dumps(worker_mem)}, borderColor: '#34d399', backgroundColor: 'transparent', tension: 0.2, pointRadius: 0 }},
                    {{ label: 'ai-engine (Python AI)', data: {json.dumps(engine_mem)}, borderColor: '#f43f5e', backgroundColor: 'transparent', tension: 0.2, pointRadius: 0 }}
                ]
            }},
            options: {{
                responsive: true,
                maintainAspectRatio: false,
                scales: {{
                    x: {{ grid: {{ color: '#334155' }}, ticks: {{ color: '#94a3b8', maxTicksLimit: 20 }} }},
                    y: {{ grid: {{ color: '#334155' }}, ticks: {{ color: '#94a3b8' }}, title: {{ display: true, text: 'RAM (MiB)', color: '#94a3b8' }} }}
                }},
                plugins: {{ legend: {{ labels: {{ color: '#f8fafc' }} }} }}
            }}
        }});

        new Chart(document.getElementById('cpuChart'), {{
            type: 'line',
            data: {{
                labels: labels,
                datasets: [
                    {{ label: 'ai-backend CPU %', data: {json.dumps(backend_cpu)}, borderColor: '#a855f7', backgroundColor: 'rgba(168, 85, 247, 0.1)', fill: true, tension: 0.2, pointRadius: 0 }}
                ]
            }},
            options: {{
                responsive: true,
                maintainAspectRatio: false,
                scales: {{
                    x: {{ grid: {{ color: '#334155' }}, ticks: {{ color: '#94a3b8', maxTicksLimit: 20 }} }},
                    y: {{ grid: {{ color: '#334155' }}, ticks: {{ color: '#94a3b8' }}, title: {{ display: true, text: 'CPU %', color: '#94a3b8' }} }}
                }},
                plugins: {{ legend: {{ labels: {{ color: '#f8fafc' }} }} }}
            }}
        }});
    </script>
</body>
</html>
"""
    with open(HTML_FILE, "w", encoding="utf-8") as f:
        f.write(html_content)

def main():
    records = []
    start_time = time.time()
    end_time = start_time + DURATION_SECONDS
    
    print(f"[{datetime.now().strftime('%H:%M:%S')}] Started 1-Hour Fixed 5k VUs Monitoring script...")

    while time.time() < end_time:
        t_str = datetime.now().strftime("%H:%M:%S")
        stats = get_docker_stats()
        if stats:
            records.append({"time": t_str, "stats": stats})
            with open(METRICS_FILE, "w", encoding="utf-8") as f:
                json.dump(records, f)
            render_html_report(records)
        time.sleep(INTERVAL_SECONDS)

if __name__ == "__main__":
    main()
