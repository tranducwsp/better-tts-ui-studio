"""
Utility module for generating dummy PCM WAV audio bytes for example TTS engine.
Hidden helper functions to keep main API handlers and gRPC server code clean and focused on contracts.
"""

import math
import random
import struct

def generate_dummy_wav(duration_sec: float = 5.0, sample_rate: int = 24000) -> bytes:
    """Returns raw 16-bit mono PCM WAV bytes of given duration_sec.

    Uses basic sine waves with harmonic overtones and fade-in/out to simulate synthetic audio.
    """
    freq = random.uniform(220, 880)
    num_samples = int(sample_rate * duration_sec)
    samples = []

    for i in range(num_samples):
        t = i / sample_rate
        val = 0.6 * math.sin(2 * math.pi * freq * t)
        val += 0.2 * math.sin(2 * math.pi * freq * 2 * t)
        val += 0.1 * math.sin(2 * math.pi * freq * 3 * t)

        # 50ms fade-in / fade-out to prevent clicks
        fade = min(i, num_samples - 1 - i, int(sample_rate * 0.05)) / (sample_rate * 0.05)
        val *= fade
        pcm = max(-32768, min(32767, int(val * 32767)))
        samples.append(pcm)

    # Build RIFF WAV header & PCM data
    data_size = num_samples * 2
    buf = bytearray()
    buf += b'RIFF'
    buf += struct.pack('<I', 36 + data_size)
    buf += b'WAVE'
    buf += b'fmt '
    buf += struct.pack('<IHHIIHH', 16, 1, 1, sample_rate, sample_rate * 2, 2, 16)
    buf += b'data'
    buf += struct.pack('<I', data_size)
    for s in samples:
        buf += struct.pack('<h', s)

    return bytes(buf)
