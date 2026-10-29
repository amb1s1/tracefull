"""Check local Markdown file links and fence balance without outbound requests."""
import re
from pathlib import Path

for path in [Path("README.md"), Path("CONTRIBUTING.md"), *Path("context").glob("*.md"), *Path("docs").glob("*.md")]:
    text = path.read_text()
    if sum(line.startswith("```") for line in text.splitlines()) % 2:
        raise SystemExit(f"Unbalanced Markdown fence: {path}")
    for target in re.findall(r"\]\(([^)]+)\)", text):
        target = target.split("#")[0]
        if target and "://" not in target and not target.startswith("mailto:"):
            if not (path.parent / target).exists():
                raise SystemExit(f"Broken local link in {path}: {target}")
print("Markdown fences and local file links passed")
