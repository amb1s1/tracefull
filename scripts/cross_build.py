"""Compile advertised candidate targets; this is not runtime verification."""
import os
import subprocess
import tempfile
from pathlib import Path

with tempfile.TemporaryDirectory(prefix="tracefull-cross-") as tmp:
    for goos in ("linux", "darwin", "windows"):
        for arch in ("amd64", "arm64"):
            env = dict(os.environ, GOOS=goos, GOARCH=arch, CGO_ENABLED="0")
            binary = Path(tmp) / f"tracefull-{goos}-{arch}"
            subprocess.run(["go", "build", "-o", str(binary), "./cmd/tracefull"], env=env, check=True)
            print(f"Compiled {goos}/{arch}; runtime not exercised by this check", flush=True)
