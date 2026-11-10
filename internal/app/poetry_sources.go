package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

// Python's standard TOML parser handles Poetry tables without a second TOML parser
// or execution of project code. Only a decision is returned, never source URLs.
const poetrySourceScript = `
import json, re, sys, tomllib
from pathlib import Path
normalize = lambda name: re.sub(r"[-_.]+", "-", name).lower()
name = normalize(sys.argv[2])
with (Path(sys.argv[1]) / "pyproject.toml").open("rb") as f:
    root = tomllib.load(f)
poetry = root.get("tool", {}).get("poetry", {})
sources = {}
for source in poetry.get("source", []):
    key = source["name"].lower()
    if key in sources:
        raise ValueError("duplicate source")
    priority = source.get("priority", "primary")
    if priority not in ("primary", "supplemental", "explicit") or "default" in source or "secondary" in source:
        raise ValueError("unsupported source priority")
    if key == "pypi" and "url" in source:
        raise ValueError("PyPI cannot have a URL override")
    sources[key] = priority
entries = []
def collect(dependencies):
    for key, value in dependencies.items():
        if normalize(key) == name:
            entries.extend(value if isinstance(value, list) else [value])
collect(poetry.get("dependencies", {}))
collect(poetry.get("dev-dependencies", {}))
for group in poetry.get("group", {}).values():
    collect(group.get("dependencies", {}))
for requirement in root.get("project", {}).get("dependencies", []):
    match = re.match(r"[A-Za-z0-9][A-Za-z0-9._-]*", requirement)
    if match and normalize(match.group()) == name:
        # Poetry's own entry enriches PEP 621 dependency metadata.
        if "@" in requirement:
            entries.append({"url": True})
        elif not entries:
            entries.append(requirement)
restricted = not entries
for entry in entries:
    if isinstance(entry, dict):
        if any(key in entry for key in ("git", "path", "url")):
            restricted = True
            continue
        source = entry.get("source")
        if source is not None:
            source = source.lower()
            if source != "pypi" and source not in sources:
                raise ValueError("undefined source")
            if source != "pypi":
                restricted = True
            continue
    elif not isinstance(entry, str):
        raise ValueError("unsupported dependency declaration")
    # Unconstrained dependencies may use any primary or supplemental source.
    if any(key != "pypi" and priority != "explicit" for key, priority in sources.items()):
        restricted = True
print(json.dumps({"restricted": restricted}))
`

func poetryRestriction(ctx context.Context, query models.Query) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "-I", "-c", poetrySourceScript, query.ProjectDir, query.Package)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot read Poetry source configuration; valid pyproject.toml and Python 3.11+ are required")
	}
	var result struct {
		Restricted bool `json:"restricted"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("invalid Poetry source decision")
	}
	if result.Restricted {
		return "Poetry project does not exclusively declare this package from PyPI; public registry and advisory requests were skipped. Assessment is unavailable.", nil
	}
	return "", nil
}
