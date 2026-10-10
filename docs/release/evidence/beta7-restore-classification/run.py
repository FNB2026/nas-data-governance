#!/usr/bin/env python3
"""Run evidence tests on an immutable Git export; never open acceptance databases."""
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
from datetime import datetime, timezone

RC = "b09166f4ef5f7a4c9fa899a3c1370163fa373e62"
HERE = Path(__file__).resolve().parent
REPO = HERE.parents[3]

def digest(data):
    return hashlib.sha256(data).hexdigest()

def now():
    return datetime.now(timezone.utc).isoformat()

def main():
    os.umask(0o077)
    archive = subprocess.check_output(["git", "archive", "--format=tar", RC], cwd=REPO)
    root = Path(tempfile.mkdtemp(prefix="ndg-beta7-classification-"))
    source = root / "frozen-source"
    source.mkdir(mode=0o700)
    with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
        for member in tar.getmembers():
            name = Path(member.name)
            if name.is_absolute() or ".." in name.parts or not (member.isfile() or member.isdir()):
                raise RuntimeError("Unsafe archive member")
        tar.extractall(source)
    original = {str(p.relative_to(source)): digest(p.read_bytes())
                for p in source.rglob("*") if p.is_file()}
    carrier = (HERE / "restore_contract_test.go.txt").read_bytes()
    target = source / "internal/app/beta7_evidence_restore_contract_test.go"
    if target.exists():
        raise RuntimeError("Evidence test name collides with frozen source")
    target.write_bytes(carrier)
    env = os.environ.copy()
    env.update({"GOWORK": "off", "GOFLAGS": "", "GOENV": "off", "GOTOOLCHAIN": "go1.26.9"})
    effective = json.loads(subprocess.check_output(
        ["go", "env", "-json", "GOOS", "GOARCH", "GOVERSION", "GOFLAGS", "GOWORK", "GOENV", "GOTOOLCHAIN"],
        cwd=source, env=env, text=True))
    start = now()
    command = ["go", "test", "-race", "-count=1", "-v", "./internal/app",
               "-run", "^TestBeta7EvidenceRestoreClassification$"]
    result = subprocess.run(command, cwd=source, env=env, capture_output=True, text=True)
    (root / "stdout.private.log").write_text(result.stdout)
    (root / "stderr.private.log").write_text(result.stderr)
    unchanged = all((source / name).is_file() and digest((source / name).read_bytes()) == sha
                    for name, sha in original.items())
    summary = {
        "rc": RC, "started_at": start, "ended_at": now(),
        "go_version": subprocess.check_output(["go", "version"], env=env, text=True).strip(),
        "effective_go_environment": effective,
        "command": command, "archive_sha256": digest(archive),
        "test_carrier_sha256": digest(carrier), "runner_sha256": digest(Path(__file__).read_bytes()),
        "frozen_files_checked": len(original), "frozen_files_unchanged": unchanged,
        "exit_code": result.returncode,
        "subcases_passed": sum(1 for line in result.stdout.splitlines()
                               if "--- PASS: TestBeta7EvidenceRestoreClassification/" in line),
        "stdout_sha256": digest(result.stdout.encode()),
        "stderr_sha256": digest(result.stderr.encode()),
        "evidence_layer": "frozen-source test binary; not signed-App runtime fields",
    }
    (root / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    # Private locator only; the public summary deliberately contains no local paths.
    Path("/tmp/ndg-beta7-classification-latest.json").write_text(json.dumps({"archive": str(root)}))
    print(json.dumps(summary, indent=2))
    if result.returncode or not unchanged or summary["subcases_passed"] != 6:
        raise SystemExit(1)

if __name__ == "__main__":
    main()
