#!/usr/bin/env python3
"""Export the immutable beta.10 RC and run test-only B/C carriers.

Usage: python3 docs/release/evidence/beta10-crash/run.py PRIVATE_OUTPUT_DIR
The exported source and all generated fixtures are independent temporary files.
No signed App, release asset, historical project or NAS source is accessed.
"""
import datetime
import hashlib
import io
import json
import pathlib
import shutil
import subprocess
import sys
import tarfile
import tempfile

RC = "c1941f84e2d1c836b57cd0750db1c2554ba6c4c9"
REPO = pathlib.Path(__file__).resolve().parents[4]
OUT = pathlib.Path(sys.argv[1]).resolve()
OUT.mkdir(mode=0o700, parents=True, exist_ok=True)
ROOT = pathlib.Path(tempfile.mkdtemp(prefix="ndg-beta10-frozen-bc-"))
raw = subprocess.check_output(["git", "archive", RC], cwd=REPO)
with tarfile.open(fileobj=io.BytesIO(raw)) as archive:
    for member in archive.getmembers():
        assert not pathlib.Path(member.name).is_absolute()
        assert ".." not in pathlib.Path(member.name).parts
    archive.extractall(ROOT)

def inventory():
    return {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest()
            for p in ROOT.rglob("*") if p.is_file() and not p.is_symlink()}

before = inventory()
carriers = ["internal/app/crash_bc_acceptance_test.go",
            "internal/adapters/wails/crash_bc_acceptance_test.go"]
hashes = {}
for name in carriers:
    shutil.copyfile(REPO / name, ROOT / name)
    hashes[name] = hashlib.sha256((ROOT / name).read_bytes()).hexdigest()
start = datetime.datetime.now(datetime.timezone.utc).isoformat()
toolchain = subprocess.check_output(["go", "version"], cwd=ROOT, text=True).strip()
command = ["go", "test", "-race", "-count=1", "-v", "-run",
           "^TestBeta10CrashBC|^TestRestoreRecoveryEvidenceMatrix$|^TestRestoreRecoveryDTOAndDurableLock$",
           "./internal/app", "./internal/adapters/wails"]
with (OUT / "race.log").open("w") as log:
    completed = subprocess.run(command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT)
after = inventory()
result = {"RC": RC, "UTC_start": start,
          "UTC_end": datetime.datetime.now(datetime.timezone.utc).isoformat(),
          "toolchain": toolchain, "command": command,
          "exit_code": completed.returncode, "frozen_files": len(before),
          "product_files_unchanged": all(after.get(k) == v for k, v in before.items()),
          "carrier_SHA256": hashes,
          "runner_SHA256": hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),
          "log_SHA256": hashlib.sha256((OUT / "race.log").read_bytes()).hexdigest(),
          "injection": "test-only panic at real CAS/no-Journal and real completed MoveFile/durable done journal boundaries; adapter separately seeds explicit new test state"}
(OUT / "result.json").write_text(json.dumps(result, indent=2) + "\n")
# This locator stays in the caller-selected private output directory.
(OUT / "export-locator.private.json").write_text(json.dumps({"export": str(ROOT)}))
print(json.dumps(result, indent=2))
if completed.returncode or not result["product_files_unchanged"]:
    sys.exit(1)
