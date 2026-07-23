import os
import torch

# GPU / CPU Threading Limits
OMP_NUM_THREADS = os.getenv("OMP_NUM_THREADS", "2")
os.environ["OMP_NUM_THREADS"] = OMP_NUM_THREADS
os.environ["MKL_NUM_THREADS"] = OMP_NUM_THREADS
os.environ["OPENBLAS_NUM_THREADS"] = OMP_NUM_THREADS

try:
    torch.set_num_threads(int(OMP_NUM_THREADS))
    torch.set_num_interop_threads(int(OMP_NUM_THREADS))
except Exception:
    pass

PORT = int(os.getenv("CORE_PORT", "8001"))
HOST = os.getenv("CORE_HOST", "0.0.0.0")

STORAGE_DIR = os.getenv("STORAGE_DIR", "storage/temp")
os.makedirs(STORAGE_DIR, exist_ok=True)
