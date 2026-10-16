#!/usr/bin/env python3
"""Private GitHub-hosted Worker probe, not a product/approval/NAS deployment.

Runs a dedicated-UID worker against real kernel peer credentials in a
disposable mount namespace, using only newly created synthetic directories.
No network, Docker, service, NAS, source cleanup or recovery-clear action.
"""
import ctypes
import grp
import importlib.util
import json
import os
from pathlib import Path
import pwd
import socket
import subprocess
import sys
import uuid

spec = importlib.util.spec_from_file_location('reader_probe_helpers', Path(__file__).with_name('managed-reader-kernel.py'))
helpers = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helpers)
run, actor, line, identity, private = helpers.run, helpers.actor, helpers.line, helpers.identity, helpers.private


def plain_actor(uid, *args):
    """Same launcher as actor() but without --no-new-privs, so the worker's own
    refusal of a process that could regain privileges is actually exercised."""
    return ['setpriv', '--reuid=' + str(uid), '--regid=' + str(uid), '--clear-groups',
            '--bounding-set=-all', '--inh-caps=-all', '--ambient-caps=-all', *args]


def main():
    assert os.geteuid() == 0 and len(sys.argv) == 2
    assert os.environ.get('GITHUB_ACTIONS') == 'true' and os.environ.get('RUNNER_ENVIRONMENT') == 'github-hosted'
    assert not Path('/usr/trim').exists(), 'never run on fnOS'
    binary = Path(sys.argv[1]).resolve(strict=True)
    assert binary.is_file() and not binary.is_symlink()
    assert ctypes.CDLL(None, use_errno=True).unshare(0x00020000) == 0
    run('mount', '--make-rprivate', '/')
    os.umask(0o077)

    used = {x.pw_uid for x in pwd.getpwall()} | {x.gr_gid for x in grp.getgrall()}
    for p in Path('/proc').iterdir():
        if p.name.isdecimal():
            try:
                for text in (p / 'status').read_text().splitlines():
                    if text.startswith(('Uid:', 'Gid:')):
                        used.update(int(v) for v in text.split()[1:])
            except (FileNotFoundError, ProcessLookupError):
                pass
    # worker: the dedicated low-privilege subject. caller: the authorized Web
    # process. outsider: every other account, which must never be admitted.
    worker, caller, outsider = [n for n in range(24000, 25000) if n not in used][:3]

    base = Path('/var/lib') / ('ndg-managed-worker-' + uuid.uuid4().hex)
    base.mkdir(mode=0o700)
    os.chown(base, worker, worker)
    results = []

    def probe_config(name, mode, run_dir, state_dir, root='', bootstrap=False, launcher=None):
        path = base / (name + '.json')
        payload = {'mode': mode, 'run': str(run_dir), 'state': str(state_dir), 'root': root,
                   'bootstrap': bootstrap, 'worker_uid': worker, 'worker_gid': worker,
                   'caller_uid': caller, 'caller_gid': caller}
        private(path, json.dumps(payload).encode(), worker)
        return path

    def launch(config, as_uid, privileged_safe=True):
        env = {**os.environ, 'NDG_MANAGED_WORKER_PROBE': str(config)}
        argv = actor(as_uid, str(binary), '-test.run=^TestManagedWorkerKernelProcess$', '-test.timeout=60s')
        if not privileged_safe:
            argv = plain_actor(as_uid, str(binary), '-test.run=^TestManagedWorkerKernelProcess$', '-test.timeout=60s')
        return subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, env=env, bufsize=0)

    def settle(process):
        if process.poll() is None:
            process.kill()
            process.communicate(timeout=10)

    def layout(name, shared=False):
        area = base / name
        area.mkdir(mode=0o700)
        run_dir, state_dir = area / 'run', area / 'state'
        for directory in (run_dir, state_dir):
            directory.mkdir(mode=0o700)
            os.chown(directory, worker, worker)
        os.chown(area, worker, worker)
        if shared:
            os.chmod(run_dir, 0o755)
        return run_dir, state_dir

    def record(case, ok, note=''):
        results.append({'case': case, 'result': 'PASS' if ok else 'FAIL', 'state': note})

    # --- 1 and 2: dedicated-UID startup with real kernel peer credentials ---
    run_dir, state_dir = layout('serve')
    process = launch(probe_config('serve', 'serve', run_dir, state_dir), worker)
    try:
        ready = line(process)
        assert ready['state'] == 'READY', 'worker did not reach READY'
        socket_path = ready['socket']
        assert Path(socket_path).parent == run_dir.resolve(), 'socket escaped the private run directory'
        identity(ready['pid'], worker)

        # An outsider UID connects first: SO_PEERCRED must refuse it.
        outside = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        outside.connect(socket_path)
        outside.close()
        # Then the authorized Web identity, which must be admitted.
        inside = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        inside.connect(socket_path)
        inside.close()

        outcome = line(process)
        assert outcome['state'] == 'PASS', 'peer authorization case failed'
        assert outcome['category'] == 'peer_authorization', outcome.get('category')
        assert outcome['outcomes'] == ['unauthorized', 'authorized'], outcome.get('outcomes')
        remaining, errors = process.communicate(timeout=15)
        assert process.returncode == 0 and errors == b'', 'serve child failed'
        record('worker_refuses_foreign_peer', True, 'SO_PEERCRED outsiders rejected')
        record('worker_accepts_authorized_peer', True, 'authorized Web identity admitted')
    finally:
        settle(process)

    # --- 3: a second worker is refused while one holds the lease ---
    hold_dir, hold_state = layout('hold')
    holder = launch(probe_config('hold', 'hold', hold_dir, hold_state), worker)
    try:
        ready = line(holder)
        assert ready['state'] == 'READY', 'lease holder did not start'
        identity(ready['pid'], worker)
        duplicate = launch(probe_config('dup', 'duplicate', hold_dir, hold_state), worker)
        try:
            outcome = line(duplicate)
            remaining, errors = duplicate.communicate(timeout=15)
            assert duplicate.returncode == 0 and errors == b'', 'duplicate child failed'
            record('worker_refuses_duplicate_lease',
                   outcome.get('category') == 'duplicate_refused', str(outcome.get('category')))
        finally:
            settle(duplicate)
        holder.stdin.write(b'release\n')
        holder.stdin.flush()
        outcome = line(holder)
        record('worker_releases_lease_cleanly', outcome.get('category') == 'lease_released', str(outcome.get('category')))
    finally:
        settle(holder)

    # --- 4: the worker verifies no-new-privs rather than assuming it ---
    nnp_dir, nnp_state = layout('nnp')
    process = launch(probe_config('nnp', 'no_new_privs', nnp_dir, nnp_state), worker, privileged_safe=False)
    try:
        outcome = line(process)
        remaining, errors = process.communicate(timeout=15)
        assert process.returncode == 0 and errors == b'', 'no-new-privs child failed'
        record('worker_requires_no_new_privs',
               outcome.get('category') == 'no_new_privs_required', str(outcome.get('category')))
    finally:
        settle(process)

    # --- 5 and 6: the worker owns a managed root, then refuses an incomplete one ---
    root_dir, root_state = layout('root')
    root = root_state / 'managed'
    process = launch(probe_config('boot', 'root', root_dir, root_state, root=str(root), bootstrap=True), worker)
    try:
        outcome = line(process)
        assert outcome['state'] == 'PASS', 'bootstrap failed'
        assert outcome['category'] == 'bootstrap', outcome.get('category')
        remaining, errors = process.communicate(timeout=15)
        assert process.returncode == 0 and errors == b'', 'bootstrap child failed'
        record('worker_bootstraps_private_root', True, 'managed root created under the dedicated identity')
    finally:
        settle(process)

    reopen = launch(probe_config('ready', 'root', root_dir, root_state, root=str(root)), worker)
    try:
        outcome = line(reopen)
        remaining, errors = reopen.communicate(timeout=15)
        assert reopen.returncode == 0 and errors == b'', 'ready child failed'
        record('worker_reopens_ready_root', outcome.get('category') == 'ready_root', str(outcome.get('category')))
    finally:
        settle(reopen)

    # Break the root the way a real interrupted bootstrap would, then confirm
    # the worker refuses it instead of adopting, repairing or clearing it.
    os.rename(root / 'control.json', base / 'control-moved-away')
    partial = launch(probe_config('partial', 'root', root_dir, root_state, root=str(root)), worker)
    try:
        outcome = line(partial)
        remaining, errors = partial.communicate(timeout=15)
        assert partial.returncode == 0 and errors == b'', 'partial child failed'
        record('worker_refuses_incomplete_root',
               outcome.get('category') == 'incomplete_refused', str(outcome.get('category')))
    finally:
        settle(partial)

    # --- 7: a shared directory is not a worker layout ---
    shared_run, shared_state = layout('shared', shared=True)
    process = launch(probe_config('shared', 'serve', shared_run, shared_state), worker)
    try:
        remaining, errors = process.communicate(timeout=30)
        denied = b'layout refused' in errors or b'private' in errors
        record('worker_refuses_shared_layout', process.returncode != 0 and denied,
               'shared 0755 run directory rejected')
    finally:
        settle(process)

    print(json.dumps({'result': 'PASS' if all(r['result'] == 'PASS' for r in results) else 'FAIL',
                      'scope': 'DISPOSABLE_NATIVE_PRIVATE_WORKER_NOT_PRODUCT_OR_NAS_OR_DEPLOYMENT_ACCEPTANCE',
                      'checks': results,
                      'uids': {'worker': worker, 'caller': caller, 'outsider': outsider},
                      'fixtures_retained_until_runner_disposal': True}))


if __name__ == '__main__':
    main()
