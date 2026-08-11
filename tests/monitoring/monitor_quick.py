#!/usr/bin/env python3
import json
import subprocess
import time
import os

containers = ['ai-backend', 'ai-engine', 'app-postgres', 'app-redis']
metrics = []
duration_sec = 210  # 3.5 minutes total (1m rest + 1.5m 5k VU + 1m rest)
interval_sec = 3
start_time = time.time()

print(f"Monitoring RAM via docker stats for {duration_sec}s...")

def parse_mem_mib(mem_str):
    parts = mem_str.split('/')[0].strip()
    if 'GiB' in parts:
        return float(parts.replace('GiB', '').strip()) * 1024
    elif 'MiB' in parts:
        return float(parts.replace('MiB', '').strip())
    elif 'KiB' in parts:
        return float(parts.replace('KiB', '').strip()) / 1024
    elif 'B' in parts:
        return float(parts.replace('B', '').strip()) / (1024 * 1024)
    return 0.0

while time.time() - start_time < duration_sec:
    t_str = time.strftime("%H:%M:%S")
    sample = {"time": t_str, "stats": {}}
    try:
        cmd = ["docker", "stats", "--no-stream", "--format", "{{.Name}}\t{{.MemUsage}}"]
        output = subprocess.check_output(cmd, text=True)
        for line in output.strip().split('\n'):
            if '\t' in line:
                name, mem = line.split('\t')
                if name in containers:
                    sample["stats"][name] = {"mem_mib": round(parse_mem_mib(mem), 2)}
    except Exception as e:
        pass

    metrics.append(sample)
    time.sleep(interval_sec)

with open("/home/amoratran/server/better-tts-ui-studio/test/metrics_quick.json", "w") as f:
    json.dump(metrics, f, indent=2)

print("Quick Monitoring complete!")
