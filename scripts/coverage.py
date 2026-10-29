"""Enforce independent parser and verdict statement coverage floors."""
import re
import subprocess
import tempfile
from pathlib import Path

with tempfile.TemporaryDirectory(prefix="tracefull-coverage-") as tmp:
    for name, packages, covered in (
        ("parsers", ["./internal/importer", "./internal/parse/..."], "./internal/parse/..."),
        ("verdicts", ["./internal/verdict"], "./internal/verdict"),
    ):
        profile = str(Path(tmp) / (name + ".out"))
        subprocess.run(["go", "test", *packages, "-coverpkg=" + covered,
                        "-coverprofile=" + profile], check=True)
        summary = subprocess.check_output(["go", "tool", "cover", "-func=" + profile], text=True)
        percent = float(re.search(r"total:.*?([\d.]+)%", summary).group(1))
        print(f"{name}: {percent:.1f}% (required: 90%)", flush=True)
        if percent < 90:
            raise SystemExit(f"{name} coverage below 90%")
