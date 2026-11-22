"""Exercise the unmodified Docker Worker with a deterministic synthetic interruption."""

import concurrent.futures
import json
import os
import signal
import subprocess
import time
import urllib.error


def await_value(call, message):
    deadline = time.monotonic() + 12
    while time.monotonic() < deadline:
        value = call()
        if value:
            return value
        time.sleep(0.05)
    raise AssertionError(message)


def verify_worker_lifecycle(base, compose, request, item_id):
    def docker(*args):
        return subprocess.check_output([*compose, *args], text=True, timeout=30).strip()

    def supervisor_mount():
        return docker("exec", "-T", "worker", "/qa-anchor", "/quarantine")

    def start():
        docker("exec", "-T", "-d", "worker", "/ndg-quarantine-candidate", "-role", "worker")

    def worker_pid():
        processes = docker("top", "worker", "-eo", "pid,args")
        matches = [
            int(row.split(maxsplit=1)[0])
            for row in processes.splitlines()[1:]
            if row.split(maxsplit=1)[-1] == "/ndg-quarantine-candidate -role worker"
        ]
        assert len(matches) <= 1, "two Workers are serving the same project"
        return matches[0] if matches else None

    def action(op, plan=None, expected=200, **fields):
        payload = {"op": "QUARANTINE_PURGE_" + op, **fields}
        if plan:
            payload.update(plan_id=plan["id"], digest=plan["digest"])
        return request(payload, expected)

    def available(call):
        try:
            return call()
        except (AssertionError, urllib.error.URLError, TimeoutError) as err:
            if isinstance(err, AssertionError) and (not err.args or err.args[0][1] != 503):
                raise
            return None

    def new_plan():
        plan = await_value(lambda: available(lambda: action("PLAN", item_id=item_id)), "Worker unavailable")
        assert action("APPROVE", plan)["state"] == "APPROVED"
        assert action("DRY_RUN", plan)["state"] == "DRY_RUN"
        return plan

    def exists(path):
        return subprocess.run(["sudo", "-n", "test", "-e", str(path)], check=False).returncode == 0

    def kill_in_execution(plan):
        pid = await_value(worker_pid, "Worker process missing")
        pending = base / "quarantine" / ".ndg-purge-stage" / "pending"
        assert not exists(pending)
        staged = pending.parent / plan["id"]
        # Pause the Worker as the staging rename returns. The plan's SQLite
        # claim and journal are committed before the staged object appears.
        trace = subprocess.Popen(
            ["sudo", "-n", "strace", "-f", "-qq", "-e", "trace=renameat2",
             "-e", "inject=renameat2:delay_exit=30s", "-p", str(pid)],
            stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
        )
        try:
            def attached():
                with open(f"/proc/{pid}/status", encoding="ascii") as status:
                    return any(line.startswith("TracerPid:") and int(line.split()[1])
                               for line in status)

            await_value(attached, "strace could not attach to the synthetic Worker")

            def execute():
                # Losing the Worker before a reply is the expected failure.
                try:
                    action("EXECUTE", plan, expected=503, confirmation="永久清理 " + plan["id"])
                except (urllib.error.URLError, TimeoutError):
                    return
            with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
                execution = pool.submit(execute)
                await_value(lambda: exists(staged), "interrupted Worker did not stage the file")
                assert exists(pending)
                assert not exists(base / "quarantine" / "synthetic.txt")
                subprocess.run(["sudo", "-n", "kill", "-KILL", str(pid)], check=True)
                execution.result(timeout=18)
        finally:
            try:
                trace.communicate(timeout=3)
            except subprocess.TimeoutExpired:
                trace.send_signal(signal.SIGTERM)
                trace.communicate(timeout=4)
        await_value(lambda: not worker_pid(), "interrupted Worker did not exit")

    mount_id = supervisor_mount()
    first = new_plan()
    duplicate = subprocess.run(
        [*compose, "exec", "-T", "worker", "/ndg-quarantine-candidate", "-role", "worker"],
        capture_output=True, timeout=15, check=False,
    )
    assert duplicate.returncode != 0
    kill_in_execution(first)
    start()
    assert supervisor_mount() == mount_id
    assert await_value(lambda: available(lambda: action("RECEIPT", first)), "Worker did not restart")["state"] == "EXECUTING"
    for _ in range(2):
        assert action("RECOVER", first)["state"] == "ROLLED_BACK"
    assert action("RECEIPT", first)["state"] == "ROLLED_BACK"
    assert exists(base / "quarantine" / "synthetic.txt")
    second = new_plan()
    kill_in_execution(second)
    docker("up", "-d", "--no-deps", "--no-build", "--pull", "never", "--force-recreate", "worker")
    new_mount_id = supervisor_mount()
    assert new_mount_id != mount_id
    start()
    assert await_value(lambda: available(lambda: action("RECEIPT", second)), "replacement Worker unavailable")["state"] == "EXECUTING"
    action("RECOVER", second, expected=409)
    assert action("RECEIPT", second)["state"] == "EXECUTING"
    assert action("RECEIPT", first)["state"] == "ROLLED_BACK"
    stage = base / "quarantine" / ".ndg-purge-stage"
    assert exists(stage / "pending")
    assert exists(stage / second["id"])
    assert not exists(base / "quarantine" / "synthetic.txt")
    print(json.dumps({"inflight_sigkill": True, "same_mount_recovered": True,
                      "changed_mount_refused": True, "old_mount_id": mount_id,
                      "new_mount_id": new_mount_id, "history_preserved": True,
                      "duplicate_worker_refused": True}))
