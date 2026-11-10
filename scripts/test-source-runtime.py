#!/usr/bin/env python3
"""Run required source-runtime and process tests; missing/skipped tests fail CI."""
import json
import os
import shutil
import subprocess
import sys

required = {
    "TestNuGetInheritedConfiguration", "TestNuGetMergeMatchesSDK",
    "TestEffectiveMavenProjectSources", "TestEffectiveGradleProjectSources",
    "TestMavenLocalDependencyIdentities", "TestMixLockSourcesWithoutEvaluation",
    "TestSourceProcesses",
}
for tool in ("go", "dotnet", "mvn", "javac", "jar", "gradle", "sh", "npm", "cargo", "ruby"):
    if shutil.which(tool) is None:
        sys.exit(f"Required runtime missing: {tool}")
if shutil.which("elixir") is None and not os.environ.get("DEPTRUST_TEST_ELIXIR_IMAGE"):
    sys.exit("Elixir or DEPTRUST_TEST_ELIXIR_IMAGE is required")
env = dict(os.environ, DEPTRUST_TEST_JVM="1", DEPTRUST_TEST_E2E="1",
           DOTNET_CLI_TELEMETRY_OPTOUT="1", DOTNET_SKIP_FIRST_TIME_EXPERIENCE="1",
           DOTNET_NOLOGO="1", DOTNET_GENERATE_ASPNET_CERTIFICATE="false")
command = ["go", "test", "-race", "-p", "1", "-count=1", "-timeout=15m", "-json",
           "./internal/app", "./e2e", "-run", "^(" + "|".join(sorted(required)) + ")$"]
passed = set()
skipped = []
with subprocess.Popen(command, env=env, stdout=subprocess.PIPE, text=True) as process:
    for line in process.stdout:
        event = json.loads(line)
        if "Output" in event:
            print(event["Output"], end="", flush=True)
        test = event.get("Test", "")
        if event.get("Action") == "pass" and test in required:
            passed.add(test)
        if event.get("Action") == "skip":
            skipped.append(test)
    status = process.wait()
if status or skipped or passed != required:
    sys.exit(f"Runtime verification failed: exit={status}, skipped={skipped}, missing={sorted(required - passed)}")
print(f"All {len(required)} required runtime/process tests passed without skips.")
