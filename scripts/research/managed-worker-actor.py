#!/usr/bin/env python3
"""Private GitHub-hosted Worker probe, not a product/approval/NAS deployment.

Runs a dedicated-UID worker against real kernel peer credentials in a
disposable mount namespace, using only newly created synthetic directories.
No network, Docker, service, NAS, source cleanup or recovery-clear action.
"""
import ctypes
import grp
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import pwd
import select
import signal
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


def envelope(op, values):
    """Build one request envelope carrying exactly the protocol keys op allows.

    The worker refuses any other key, so the envelope must match the fixed
    field contract for the op. STATUS has no fields at all; SEAL/PURGE/INSPECT
    differ in which of object_id, size, sha256, authorization_sha256, device
    and inode they accept.
    """
    if op == 'STATUS':
        keys = ()
    elif op == 'SEAL':
        keys = ('object_id', 'size', 'sha256', 'authorization_sha256')
    elif op == 'PURGE':
        keys = ('object_id', 'authorization_sha256', 'size', 'sha256', 'device', 'inode')
    else:  # INSPECT
        keys = ('object_id', 'size', 'sha256', 'device', 'inode')
    return {'op': op, 'fields': {k: values[k] for k in keys}}


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

    # Traversable but not listable: a peer must be able to reach its own probe
    # input and the socket by exact path, while every probe file stays private
    # to the identity that runs it. A sealed base would stop an outsider at the
    # filesystem and the peer-credential check would never be what refused it.
    def directory(path, mode, owner=None):
        """Create a directory with an exact mode and prove it.

        mkdir's mode argument is filtered by the process umask, which is 0o077
        here. A directory meant to be reachable would therefore silently come
        out sealed at 0700: the peer would be stopped by the filesystem instead
        of by its credentials, and the case would prove nothing while still
        looking healthy. chmod is not filtered, so the mode is set explicitly
        and then read back.
        """
        path.mkdir(mode=mode)
        os.chmod(path, mode)
        assert os.stat(path).st_mode & 0o7777 == mode, 'directory mode not exact: ' + str(path)
        if owner is not None:
            os.chown(path, owner, owner)
        return path

    base = Path('/var/lib') / ('ndg-managed-worker-' + uuid.uuid4().hex)
    directory(base, 0o711, worker)
    results = []

    def probe_config(name, mode, run_dir, state_dir, root='', bootstrap=False, socket_path='', owner=None, envelopes=(), auth=''):
        """owner defaults to the worker, but a dialer must be able to read its
        own config, so each probe file is owned by the identity that runs it."""
        path = base / (name + '.json')
        payload = {'mode': mode, 'run': str(run_dir), 'state': str(state_dir), 'root': root,
                   'socket': socket_path, 'envelopes': list(envelopes), 'auth': auth,
                   'bootstrap': bootstrap,
                   'worker_uid': worker, 'worker_gid': worker,
                   'caller_uid': caller, 'caller_gid': caller}
        private(path, json.dumps(payload).encode(), worker if owner is None else owner)
        return path

    def handshake(process, label):
        """Read one probe line, surfacing the child's own stderr when it fails
        so a refusal is never mistaken for a lost handshake."""
        if not select.select([process.stdout], [], [], 20)[0]:
            settle(process)
            raise AssertionError(label + ': probe handshake timeout')
        raw = process.stdout.readline(8193)
        if not raw:
            errors = process.stderr.read().decode('utf-8', 'replace') if process.stderr else ''
            settle(process)
            raise AssertionError(label + ': no probe output; child stderr=' + errors[-800:])
        assert len(raw) <= 8192 and raw.endswith(b'\n'), label + ': probe output bound'
        try:
            return json.loads(raw)
        except ValueError:
            # A failing probe reports its own reason on stdout. Without this the
            # first line would only ever surface as an opaque JSON error, which
            # is indistinguishable from a harness bug.
            if process.poll() is None:
                process.kill()
            remaining, errors = process.communicate(timeout=10)
            raise AssertionError(label + ': probe output not JSON; stdout=' +
                                 repr(raw[:200] + remaining[:600]) + '; child stderr=' +
                                 errors.decode('utf-8', 'replace')[-800:])

    def reach(uid, flag, path):
        """Prove the identity can reach the path itself, so that a filesystem
        block can never masquerade as a peer-credential refusal."""
        checked = subprocess.run(['setpriv', '--reuid=' + str(uid), '--regid=' + str(uid),
                                  '--clear-groups', '--no-new-privs', 'test',
                                  '-' + flag, str(path)],
                                 capture_output=True, timeout=15)
        assert checked.returncode == 0, (str(uid) + ' cannot reach ' + str(path) +
                                         ' (-' + flag + '): ' +
                                         checked.stderr.decode('utf-8', 'replace')[-200:])

    def launch(config, as_uid, privileged_safe=True):
        env = {**os.environ, 'NDG_MANAGED_WORKER_PROBE': str(config)}
        argv = actor(as_uid, str(binary), '-test.run=^TestManagedWorkerKernelProcess$', '-test.timeout=60s')
        if not privileged_safe:
            argv = plain_actor(as_uid, str(binary), '-test.run=^TestManagedWorkerKernelProcess$', '-test.timeout=60s')
        return subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, env=env, bufsize=0)

    def launch_privileged(config):
        """Launch the probe as root with full capabilities, but with no-new-privs
        already established: the strongest impostor for the dedicated worker
        subject, with the gate that would otherwise fire first satisfied so the
        subject gate is what refuses."""
        env = {**os.environ, 'NDG_MANAGED_WORKER_PROBE': str(config)}
        return subprocess.Popen(['setpriv', '--no-new-privs', str(binary),
                                 '-test.run=^TestManagedWorkerKernelProcess$',
                                 '-test.timeout=60s'],
                                stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, env=env, bufsize=0)

    def settle(process):
        if process.poll() is None:
            process.kill()
            process.communicate(timeout=10)

    def layout(name, foreign_writable=False):
        # The run directory is traversable on purpose: a peer must be able to
        # reach the socket before its credentials can be examined. It stays
        # unwritable by anyone but the worker. State is strictly private. The
        # area above them is traversable but not listable for the same reason.
        area = base / name
        directory(area, 0o711, worker)
        run_dir, state_dir = area / 'run', area / 'state'
        directory(run_dir, 0o755, worker)
        directory(state_dir, 0o700, worker)
        if foreign_writable:
            os.chmod(run_dir, 0o777)
        return run_dir, state_dir

    def record(case, ok, note=''):
        results.append({'case': case, 'result': 'PASS' if ok else 'FAIL', 'state': note})

    # --- 1 and 2: dedicated-UID startup with real kernel peer credentials ---
    run_dir, state_dir = layout('serve')
    process = launch(probe_config('serve', 'serve', run_dir, state_dir), worker)
    try:
        ready = handshake(process, 'serve')
        assert ready['state'] == 'READY', 'worker did not reach READY'
        socket_path = ready['socket']
        assert Path(socket_path).parent == run_dir.resolve(), 'socket escaped the private run directory'
        identity(ready['pid'], worker)

        # The peer identity only means something when the connecting process
        # really carries it, so each dial runs under its own UID: an outsider
        # first, which SO_PEERCRED must refuse, then the authorized Web UID.
        for label, uid in (('outsider', outsider), ('caller', caller)):
            config = probe_config('dial-' + label, 'dial', run_dir, state_dir,
                                  socket_path=socket_path, owner=uid)
            # Both must hold before the dial means anything: the peer reads its
            # own probe input, and it can reach and write the socket. Otherwise
            # the filesystem would refuse it and SO_PEERCRED would decide nothing.
            reach(uid, 'r', config)
            reach(uid, 'w', socket_path)
            dialer = launch(config, uid)
            try:
                outcome = handshake(dialer, 'dial-' + label)
                remaining, errors = dialer.communicate(timeout=20)
                assert dialer.returncode == 0 and errors == b'', 'dial ' + label + ' failed'
                assert outcome['uid'] == uid, 'dial ran under the wrong identity'
            finally:
                settle(dialer)

        outcome = handshake(process, 'serve-outcome')
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
        ready = handshake(holder, 'hold')
        assert ready['state'] == 'READY', 'lease holder did not start'
        identity(ready['pid'], worker)
        duplicate = launch(probe_config('dup', 'duplicate', hold_dir, hold_state), worker)
        try:
            outcome = handshake(duplicate, 'duplicate')
            remaining, errors = duplicate.communicate(timeout=15)
            assert duplicate.returncode == 0 and errors == b'', 'duplicate child failed'
            record('worker_refuses_duplicate_lease',
                   outcome.get('category') == 'duplicate_refused', str(outcome.get('category')))
        finally:
            settle(duplicate)
        holder.stdin.write(b'release\n')
        holder.stdin.flush()
        outcome = handshake(holder, 'hold-release')
        record('worker_releases_lease_cleanly', outcome.get('category') == 'lease_released', str(outcome.get('category')))
    finally:
        settle(holder)

    # --- 4: the worker verifies no-new-privs rather than assuming it ---
    nnp_dir, nnp_state = layout('nnp')
    process = launch(probe_config('nnp', 'no_new_privs', nnp_dir, nnp_state), worker, privileged_safe=False)
    try:
        outcome = handshake(process, 'no-new-privs')
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
        outcome = handshake(process, 'bootstrap')
        assert outcome['state'] == 'PASS', 'bootstrap failed'
        assert outcome['category'] == 'bootstrap', outcome.get('category')
        remaining, errors = process.communicate(timeout=15)
        assert process.returncode == 0 and errors == b'', 'bootstrap child failed'
        record('worker_bootstraps_private_root', True, 'managed root created under the dedicated identity')
    finally:
        settle(process)

    reopen = launch(probe_config('ready', 'root', root_dir, root_state, root=str(root)), worker)
    try:
        outcome = handshake(reopen, 'ready-root')
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
        outcome = handshake(partial, 'partial-root')
        remaining, errors = partial.communicate(timeout=15)
        assert partial.returncode == 0 and errors == b'', 'partial child failed'
        record('worker_refuses_incomplete_root',
               outcome.get('category') == 'incomplete_refused',
               'stage=' + str(outcome.get('stage')) + ' category=' + str(outcome.get('category')))
    finally:
        settle(partial)

    # --- 8: SIGKILL mid-operation leaves a root the next worker must refuse ---
    crash_run, crash_state = layout('crash')
    crash_root = crash_state / 'managed'
    boot = launch(probe_config('crash-boot', 'root', crash_run, crash_state,
                               root=str(crash_root), bootstrap=True), worker)
    try:
        outcome = handshake(boot, 'crash-bootstrap')
        remaining, errors = boot.communicate(timeout=20)
        assert boot.returncode == 0 and errors == b'', 'crash bootstrap failed'
        assert outcome['category'] == 'bootstrap', outcome.get('category')
    finally:
        settle(boot)

    # Start a real SealCopy and kill the worker from inside the copy, after the
    # root Pending marker and the catalog intent are already durable.
    victim = launch(probe_config('crash-victim', 'pending', crash_run, crash_state,
                                 root=str(crash_root)), worker)
    try:
        ready = handshake(victim, 'crash-victim')
        assert ready['state'] == 'READY', 'pending worker did not start'
        identity(ready['pid'], worker)
        os.kill(ready['pid'], signal.SIGKILL)
        victim.wait(timeout=20)
        assert victim.returncode == -signal.SIGKILL or victim.returncode < 0, 'victim was not killed'
        record('worker_killed_mid_operation', True, 'SIGKILL delivered inside SealCopy')
    finally:
        settle(victim)

    # The lease is released by the kernel on death, so a new worker can take
    # it; what must not happen is that new worker opening the pending root.
    after = launch(probe_config('crash-after', 'root', crash_run, crash_state,
                                root=str(crash_root)), worker)
    try:
        outcome = handshake(after, 'crash-after')
        remaining, errors = after.communicate(timeout=20)
        assert after.returncode == 0 and errors == b'', 'post-crash child failed'
        record('worker_refuses_pending_root_after_crash',
               outcome.get('category') == 'incomplete_refused',
               'stage=' + str(outcome.get('stage')) + ' category=' + str(outcome.get('category')))
    finally:
        settle(after)

    # --- 9: a replaced managed-root directory is not the recorded one ---
    swap_run, swap_state = layout('swap')
    swap_root = swap_state / 'managed'
    boot = launch(probe_config('swap-boot', 'root', swap_run, swap_state,
                               root=str(swap_root), bootstrap=True), worker)
    try:
        outcome = handshake(boot, 'swap-bootstrap')
        remaining, errors = boot.communicate(timeout=20)
        assert boot.returncode == 0 and errors == b'', 'swap bootstrap failed'
    finally:
        settle(boot)
    # Replace the objects directory with a different one: the child identity
    # stored in control.json no longer describes what is on disk.
    #
    # The original is moved out of the root instead of being deleted. Deleting
    # it frees the inode, and a filesystem may hand that same inode straight
    # back to the replacement, which leaves the recorded identity unchanged: the
    # worker would then be right to open the root and the case would prove
    # nothing while looking like a failure. Keeping the original alive makes the
    # new identity necessarily different, and the assertion below says so.
    original = os.stat(swap_root / 'objects')
    os.rename(swap_root / 'objects', swap_state / 'objects-replaced')
    directory(swap_root / 'objects', 0o700, worker)
    replaced = os.stat(swap_root / 'objects')
    assert (original.st_dev, original.st_ino) != (replaced.st_dev, replaced.st_ino), \
        'replacement kept the recorded identity; the case cannot prove anything'
    swapped = launch(probe_config('swap-after', 'root', swap_run, swap_state,
                                  root=str(swap_root)), worker)
    try:
        outcome = handshake(swapped, 'swap-after')
        remaining, errors = swapped.communicate(timeout=20)
        assert swapped.returncode == 0 and errors == b'', 'post-swap child failed'
        record('worker_refuses_replaced_root',
               outcome.get('category') == 'incomplete_refused',
               'stage=' + str(outcome.get('stage')) + ' category=' + str(outcome.get('category')))
    finally:
        settle(swapped)

    # --- 10: a mount change over the root is detected, not followed ---
    mount_run, mount_state = layout('mount')
    mount_root = mount_state / 'managed'
    boot = launch(probe_config('mount-boot', 'root', mount_run, mount_state,
                               root=str(mount_root), bootstrap=True), worker)
    try:
        outcome = handshake(boot, 'mount-bootstrap')
        remaining, errors = boot.communicate(timeout=20)
        assert boot.returncode == 0 and errors == b'', 'mount bootstrap failed'
    finally:
        settle(boot)
    decoy = mount_state / 'decoy'
    directory(decoy, 0o700, worker)
    run('mount', '--bind', str(decoy), str(mount_root))
    remounted = launch(probe_config('mount-after', 'root', mount_run, mount_state,
                                    root=str(mount_root)), worker)
    try:
        outcome = handshake(remounted, 'mount-after')
        remaining, errors = remounted.communicate(timeout=20)
        assert remounted.returncode == 0 and errors == b'', 'post-mount child failed'
        record('worker_refuses_changed_mount',
               outcome.get('category') == 'incomplete_refused',
               'stage=' + str(outcome.get('stage')) + ' category=' + str(outcome.get('category')))
    finally:
        settle(remounted)
        run('umount', str(mount_root))

    # --- 11: a foreign-writable run directory is not a worker layout ---
    shared_run, shared_state = layout('shared', foreign_writable=True)
    process = launch(probe_config('shared', 'serve', shared_run, shared_state), worker)
    try:
        remaining, errors = process.communicate(timeout=30)
        # A failing Go test reports on stdout, never on stderr.
        report = remaining + errors
        denied = any(token in report for token in
                     (b'layout_refused', b'layout refused', b'private'))
        record('worker_refuses_shared_layout', process.returncode != 0 and denied,
               'shared run directory rejected: ' +
               report.decode('utf-8', 'replace').strip().replace('\n', ' | ')[-160:])
    finally:
        settle(process)

    # --- 12: the root path runs the same gates, and refuses before the root exists ---
    gate_run, gate_state = layout('gate')
    gate_root = gate_state / 'managed'
    impostor = launch_privileged(probe_config('gate', 'root', gate_run, gate_state,
                                              root=str(gate_root), bootstrap=True))
    try:
        outcome = handshake(impostor, 'root-under-root')
        remaining, errors = impostor.communicate(timeout=20)
        assert impostor.returncode == 0 and errors == b'', 'impostor child failed'
        record('worker_refuses_root_under_foreign_subject',
               outcome.get('category') == 'subject_refused',
               'category=' + str(outcome.get('category')))
        # The gate has to run before the root is touched. An impostor that had
        # reached bootstrap would already have left the directory behind, and
        # the managed filesystem layer would also refuse root -- so the category
        # above is what proves the worker's own gate stopped it, not that one.
        # No-new-privs is already established for this process, so the subject
        # gate is the first one that can refuse.
        assert not gate_root.exists(), 'the root was created before the subject gate refused'
    finally:
        settle(impostor)

    # --- 13: a second worker cannot take a managed root another one holds ---
    dup_run, dup_state = layout('rootdup')
    dup_root = dup_state / 'managed'
    holder = launch(probe_config('rootdup-hold', 'pending', dup_run, dup_state,
                                 root=str(dup_root), bootstrap=True), worker)
    try:
        ready = handshake(holder, 'rootdup-hold')
        assert ready['state'] == 'READY', 'root holder did not start'
        identity(ready['pid'], worker)
        duplicate = launch(probe_config('rootdup-dup', 'root', dup_run, dup_state,
                                        root=str(dup_root)), worker)
        try:
            outcome = handshake(duplicate, 'rootdup-duplicate')
            remaining, errors = duplicate.communicate(timeout=20)
            assert duplicate.returncode == 0 and errors == b'', 'duplicate root child failed'
            record('worker_refuses_second_root_holder',
                   outcome.get('category') == 'duplicate_refused',
                   'category=' + str(outcome.get('category')))
            # The refused worker must not have opened, recovered or cleared
            # anything: the held root keeps the control record it was built with.
            assert (dup_root / 'control.json').exists(), 'the held root lost its control record'
        finally:
            settle(duplicate)
    finally:
        settle(holder)

    # --- 14: the root path refuses a worker the launcher left privilegeable ---
    nnp_root_run, nnp_root_state = layout('nnproot')
    nnp_root = nnp_root_state / 'managed'
    process = launch(probe_config('nnproot', 'root', nnp_root_run, nnp_root_state,
                                  root=str(nnp_root), bootstrap=True), worker,
                     privileged_safe=False)
    try:
        outcome = handshake(process, 'root-without-no-new-privs')
        remaining, errors = process.communicate(timeout=20)
        assert process.returncode == 0 and errors == b'', 'no-new-privs root child failed'
        record('worker_refuses_root_without_no_new_privs',
               outcome.get('category') == 'no_new_privs_required',
               'category=' + str(outcome.get('category')))
        # Setting no-new-privs from inside the process only covers the calling
        # thread, and a Go worker already has others by then, so the worker
        # cannot repair a launcher that left it out. The gate has to refuse, and
        # it has to refuse before the root exists.
        assert not nnp_root.exists(), 'the root was created before no-new-privs was required'
    finally:
        settle(process)

    # --- 15: an authorized caller still cannot reach SEAL, PURGE or INSPECT ---
    # Reaching the socket is not the same permission as deleting an object, so
    # the operations are sent by the authorized peer itself and each refusal is
    # witnessed by the caller that would have performed it. Each envelope is a
    # complete, well-formed business request (real op, full protocol field set,
    # genuine-looking identity), so the refusal that comes back is attributed to
    # the authorization gate rather than to request validation.
    synth = {'object_id': 'a' * 64, 'size': 1, 'sha256': 'b' * 64,
             'authorization_sha256': 'c' * 64, 'device': 1, 'inode': 2}
    envelopes = [envelope(op, synth) for op in ('SEAL', 'PURGE', 'INSPECT', 'STATUS')]
    disp_run, disp_state = layout('dispatch')
    process = launch(probe_config('dispatch', 'dispatch', disp_run, disp_state, envelopes=envelopes), worker)
    try:
        ready = handshake(process, 'dispatch')
        assert ready['state'] == 'READY', 'dispatch worker did not start'
        identity(ready['pid'], worker)
        sender = launch(probe_config('send', 'send', disp_run, disp_state,
                                     socket_path=ready['socket'], envelopes=envelopes, owner=caller), caller)
        try:
            reach(caller, 'w', ready['socket'])
            outcome = handshake(sender, 'send')
            remaining, errors = sender.communicate(timeout=20)
            assert sender.returncode == 0 and errors == b'', 'send child failed'
            for entry in outcome['outcomes']:
                op, _, body = entry.partition(':')
                answer = json.loads(body)
                assert answer['ok'] is (op == 'STATUS'), entry
        finally:
            settle(sender)
        outcome = handshake(process, 'dispatch-outcome')
        assert outcome['state'] == 'PASS', 'dispatch case failed'
        assert outcome['category'] == 'dispatch', outcome.get('category')
        assert outcome['outcomes'] == ['SEAL:authorization_refused',
                                       'PURGE:authorization_refused',
                                       'INSPECT:authorization_refused',
                                       'STATUS:executed'], outcome.get('outcomes')
        assert outcome['executable'] == ['PING', 'STATUS'], outcome.get('executable')
        remaining, errors = process.communicate(timeout=15)
        assert process.returncode == 0 and errors == b'', 'dispatch child failed'
        record('worker_refuses_dangerous_ops_for_authorized_peer', True,
               'SEAL/PURGE/INSPECT refused over an authorized connection, by authorization')
    finally:
        settle(process)

    # --- 18: a malformed approval digest is refused before anything is written ---
    auth_run, auth_state = layout('auth')
    auth_root = auth_state / 'managed'
    boot = launch(probe_config('auth-boot', 'root', auth_run, auth_state,
                               root=str(auth_root), bootstrap=True), worker)
    try:
        outcome = handshake(boot, 'auth-bootstrap')
        remaining, errors = boot.communicate(timeout=20)
        assert boot.returncode == 0 and errors == b'', 'auth bootstrap failed'
        assert outcome['category'] == 'bootstrap', outcome.get('category')
    finally:
        settle(boot)
    before = sorted(p.name for p in (auth_root / 'objects').iterdir())
    process = launch(probe_config('auth-refused', 'seal_refused', auth_run, auth_state,
                                  root=str(auth_root)), worker)
    try:
        outcome = handshake(process, 'seal-refused')
        remaining, errors = process.communicate(timeout=20)
        assert process.returncode == 0 and errors == b'', 'seal-refused child failed'
        record('worker_refuses_malformed_approval_digest',
               outcome.get('category') == 'malformed_authorization_refused',
               'category=' + str(outcome.get('category')))
        # A refusal must not write: the object inventory is unchanged, and the
        # root still opens as Ready rather than carrying a pending intent.
        after = sorted(p.name for p in (auth_root / 'objects').iterdir())
        assert before == after, 'a refused copy changed the object inventory'
    finally:
        settle(process)

    # --- 19: the approval digest that is accepted is durably recorded ---
    # The primitive records the digest for audit; it does not validate approval,
    # so the record itself is what has to exist. The digest is chosen here and
    # read back out of the control record while the copy is still in flight.
    audit_run, audit_state = layout('audit')
    audit_root = audit_state / 'managed'
    boot = launch(probe_config('audit-boot', 'root', audit_run, audit_state,
                               root=str(audit_root), bootstrap=True), worker)
    try:
        outcome = handshake(boot, 'audit-bootstrap')
        remaining, errors = boot.communicate(timeout=20)
        assert boot.returncode == 0 and errors == b'', 'audit bootstrap failed'
    finally:
        settle(boot)
    digest = hashlib.sha256(b'synthetic approval decision').hexdigest()
    victim = launch(probe_config('audit-victim', 'pending', audit_run, audit_state,
                                 root=str(audit_root), auth=digest), worker)
    try:
        ready = handshake(victim, 'audit-victim')
        assert ready['state'] == 'READY', 'audit copy did not start'
        control = json.loads((audit_root / 'control.json').read_text())
        recorded = control.get('pending', {}).get('authorization_sha256')
        assert recorded == digest, 'approval digest not recorded: ' + str(recorded)
        record('worker_records_approval_digest_for_audit', True,
               'authorization_sha256 recorded in the durable intent')
    finally:
        settle(victim)

    # --- 20 and 21: the stored object identity is accepted, and only that ---
    # The reader verifies supplied evidence, so the positive control matters as
    # much as the refusals: a verifier that refused everything would record
    # every tampered variant as a pass. The untampered proof is therefore
    # inspected inside the same process, before the tampering and again after
    # it, and the driver independently re-reads the sealed bytes around both.
    id_run, id_state = layout('identity')
    id_root = id_state / 'managed'
    boot = launch(probe_config('id-boot', 'root', id_run, id_state,
                               root=str(id_root), bootstrap=True), worker)
    try:
        outcome = handshake(boot, 'identity-bootstrap')
        remaining, errors = boot.communicate(timeout=20)
        assert boot.returncode == 0 and errors == b'', 'identity bootstrap failed'
        assert outcome['category'] == 'bootstrap', outcome.get('category')
    finally:
        settle(boot)
    seal = launch(probe_config('id-seal', 'seal', id_run, id_state, root=str(id_root)), worker)
    try:
        outcome = handshake(seal, 'identity-seal')
        remaining, errors = seal.communicate(timeout=20)
        assert seal.returncode == 0 and errors == b'', 'identity seal failed'
        assert outcome['category'] == 'sealed', outcome.get('category')
        object_id = outcome['object_id']
        sealed_hash = outcome['sha256']
    finally:
        settle(seal)
    object_path = id_root / 'objects' / object_id
    sealed_bytes = object_path.read_bytes()
    assert hashlib.sha256(sealed_bytes).hexdigest() == sealed_hash, \
        'the sealed bytes on disk are not what the seal recorded'

    reader = launch(probe_config('id-read', 'reader', id_run, id_state, root=str(id_root)), worker)
    try:
        outcome = handshake(reader, 'identity-read')
        remaining, errors = reader.communicate(timeout=20)
        assert reader.returncode == 0 and errors == b'', 'identity reader failed'
        record('worker_accepts_stored_object_proof',
               outcome.get('category') == 'object_identity_accepted' and
               outcome.get('outcomes') == ['accepted'],
               'category=' + str(outcome.get('category')))
    finally:
        settle(reader)
    assert object_path.read_bytes() == sealed_bytes, \
        'inspection modified the sealed object'

    refused = launch(probe_config('id-refused', 'reader_refused', id_run, id_state,
                                  root=str(id_root)), worker)
    try:
        outcome = handshake(refused, 'identity-refused')
        remaining, errors = refused.communicate(timeout=20)
        assert refused.returncode == 0 and errors == b'', 'identity refusal child failed'
        record('worker_refuses_tampered_object_proof',
               outcome.get('category') == 'object_identity_refused' and
               outcome.get('outcomes') == ['accepted', 'digest_refused', 'size_refused',
                                           'inode_refused', 'root_refused', 'object_id_refused',
                                           'accepted_again'],
               'outcomes=' + ','.join(outcome.get('outcomes') or []))
    finally:
        settle(refused)
    assert object_path.read_bytes() == sealed_bytes, \
        'the refusals modified the sealed object'
    # Nothing durable happened either: the root is still ready, not pending.
    check = launch(probe_config('id-check', 'root', id_run, id_state, root=str(id_root)), worker)
    try:
        outcome = handshake(check, 'identity-check')
        remaining, errors = check.communicate(timeout=20)
        assert check.returncode == 0 and errors == b'', 'identity check child failed'
        assert outcome.get('category') == 'ready_root', outcome.get('category')
    finally:
        settle(check)

    # --- 22: a complete, genuine business envelope is refused by authorization ---
    # The envelopes below carry a REAL, freshly-sealed object identity (object_id,
    # size, sha256, device, inode from the seal) plus a genuine approval digest.
    # The caller is the authorized Web identity, over a real authorized
    # connection. Because the object actually exists and is correctly named, the
    # refusal cannot be "the object was not found": it is the authorization gate,
    # in the same worker that would otherwise dispatch SEAL/PURGE/INSPECT. STATUS
    # still executes, which is the positive control proving the path accepts at
    # least one operation, so the dangerous-op refusals are not a connection
    # failure. Cases 20-21 prove this very identity is one the reader accepts.
    env_run, env_state = layout('envelope')
    env_root = env_state / 'managed'
    boot = launch(probe_config('env-boot', 'root', env_run, env_state,
                               root=str(env_root), bootstrap=True), worker)
    try:
        outcome = handshake(boot, 'env-bootstrap')
        remaining, errors = boot.communicate(timeout=20)
        assert boot.returncode == 0 and errors == b'', 'envelope bootstrap failed'
        assert outcome['category'] == 'bootstrap', outcome.get('category')
    finally:
        settle(boot)
    seal = launch(probe_config('env-seal', 'seal', env_run, env_state, root=str(env_root)), worker)
    try:
        outcome = handshake(seal, 'env-seal')
        remaining, errors = seal.communicate(timeout=20)
        assert seal.returncode == 0 and errors == b'', 'envelope seal failed'
        assert outcome['category'] == 'sealed', outcome.get('category')
        object_id = outcome['object_id']
        sealed_hash = outcome['sha256']
    finally:
        settle(seal)
    object_path = env_root / 'objects' / object_id
    sealed_bytes = object_path.read_bytes()
    assert hashlib.sha256(sealed_bytes).hexdigest() == sealed_hash, \
        'the envelope sealed bytes on disk are not what the seal recorded'
    # Build a genuinely complete envelope from the stored object-proof: the
    # worker's reader would accept this very identity (cases 20-21), so the
    # authorization refusal is not a validation or object-not-found failure.
    proof = json.loads((env_state / 'object-proof.json').read_text())
    real = {'object_id': proof['object_id'], 'size': proof['size'],
            'sha256': proof['sha256'], 'authorization_sha256': 'c' * 64,
            'device': proof['device'], 'inode': proof['inode']}
    envelopes = [envelope(op, real) for op in ('SEAL', 'PURGE', 'INSPECT', 'STATUS')]
    process = launch(probe_config('env-dispatch', 'dispatch', env_run, env_state,
                                 envelopes=envelopes), worker)
    try:
        ready = handshake(process, 'env-dispatch')
        assert ready['state'] == 'READY', 'envelope dispatch worker did not start'
        identity(ready['pid'], worker)
        sender = launch(probe_config('env-send', 'send', env_run, env_state,
                                     socket_path=ready['socket'], envelopes=envelopes,
                                     owner=caller), caller)
        try:
            reach(caller, 'w', ready['socket'])
            outcome = handshake(sender, 'env-send')
            remaining, errors = sender.communicate(timeout=20)
            assert sender.returncode == 0 and errors == b'', 'envelope send child failed'
            for entry in outcome['outcomes']:
                op, _, body = entry.partition(':')
                answer = json.loads(body)
                assert answer['ok'] is (op == 'STATUS'), entry
        finally:
            settle(sender)
        outcome = handshake(process, 'env-dispatch-outcome')
        assert outcome['state'] == 'PASS', 'envelope dispatch case failed'
        assert outcome['category'] == 'dispatch', outcome.get('category')
        assert outcome['outcomes'] == ['SEAL:authorization_refused',
                                       'PURGE:authorization_refused',
                                       'INSPECT:authorization_refused',
                                       'STATUS:executed'], outcome.get('outcomes')
        assert outcome['executable'] == ['PING', 'STATUS'], outcome.get('executable')
        remaining, errors = process.communicate(timeout=15)
        assert process.returncode == 0 and errors == b'', 'envelope dispatch child failed'
        record('worker_refuses_complete_envelope_for_authorized_peer', True,
               'real object identity + approval digest refused by authorization gate')
    finally:
        settle(process)
    # Authorization performed no object operation: the sealed bytes are unchanged
    # and the root is still Ready rather than carrying a pending intent.
    assert object_path.read_bytes() == sealed_bytes, \
        'the authorization refusal modified the sealed object'
    check = launch(probe_config('env-check', 'root', env_run, env_state, root=str(env_root)), worker)
    try:
        outcome = handshake(check, 'env-check')
        remaining, errors = check.communicate(timeout=20)
        assert check.returncode == 0 and errors == b'', 'envelope check child failed'
        assert outcome.get('category') == 'ready_root', outcome.get('category')
    finally:
        settle(check)

    # --- 23: the business authorization gate answers a real authorized connection ---
    # The worker now wires its dispatch path to the durable authorization
    # store. One legally approved envelope reaches an eligible internal
    # verdict (and executes nothing); forged, drifted, unplanned and revoked
    # envelopes get typed denials. Quarantine item ids are deterministic
    # (sha256 over plan id, action index and target path), so the driver can
    # compose every envelope before the worker seeds its own store. The worker
    # seeds exactly one eligible object and one revoked-plan object; the
    # seeded bytes must be byte-identical after the whole exchange.
    authz_run, authz_state = layout('authzgate')
    plan_id = 'purge-kernel-authorize'
    revoked_plan_id = 'purge-kernel-revoked'
    content = b'gate-object-payload'
    revoked_content = b'gate-object-revoked-payload'
    digest = 'a' * 64
    revoked_digest = 'c' * 64

    def deterministic_item_id(pid, target):
        # Mirrors store.quarantineItemID: sha256(planID \0 actionIndex \0 targetPath),
        # first 24 hex chars, prefixed with 'q-'.
        return 'q-' + hashlib.sha256(f'{pid}\x00{0}\x00{target}'.encode()).hexdigest()[:24]

    object_path = authz_state / 'quarantine' / 'gate-object'
    revoked_object_path = authz_state / 'quarantine' / 'gate-object-revoked'
    item = deterministic_item_id(plan_id, str(object_path))
    revoked_item = deterministic_item_id(revoked_plan_id, str(revoked_object_path))

    legal = {'plan_id': plan_id, 'object_id': item, 'sha256': hashlib.sha256(content).hexdigest(),
             'authorization_sha256': digest, 'size': len(content), 'device': 1, 'inode': 2}

    def purged(fields):
        return {'op': 'PURGE', 'fields': fields}

    def without(d, key):
        rest = dict(d)
        rest.pop(key)
        return rest

    authz_envelopes = [
        purged(legal),
        purged({**legal, 'authorization_sha256': 'b' * 64}),
        purged({**legal, 'size': len(content) + 1}),
        purged({**legal, 'object_id': 'e' * 64}),
        purged(without(legal, 'plan_id')),
        purged({**legal, 'plan_id': 'purge-does-not-exist'}),
        purged({'plan_id': revoked_plan_id, 'object_id': revoked_item,
                'sha256': hashlib.sha256(revoked_content).hexdigest(),
                'authorization_sha256': revoked_digest, 'size': len(revoked_content),
                'device': 1, 'inode': 2}),
        {'op': 'PING', 'fields': {}},
        {'op': 'STATUS', 'fields': {}},
    ]
    expected_authz_outcomes = [
        'PURGE:authorized_verdict',
        'PURGE:denied:tampered',
        'PURGE:denied:wrong_object',
        'PURGE:denied:wrong_object',
        'PURGE:denied:wrong_plan',
        'PURGE:denied:wrong_plan',
        'PURGE:denied:revoked',
        'PING:executed',
        'STATUS:executed',
    ]
    authz_process = launch(probe_config('authzgate', 'authorize', authz_run, authz_state,
                                        envelopes=authz_envelopes), worker)
    try:
        ready = handshake(authz_process, 'authzgate')
        assert ready['state'] == 'READY', 'authorize worker did not start'
        identity(ready['pid'], worker)
        sender = launch(probe_config('authzgate-send', 'send', authz_run, authz_state,
                                     socket_path=ready['socket'], envelopes=authz_envelopes,
                                     owner=caller), caller)
        try:
            reach(caller, 'w', ready['socket'])
            sent = handshake(sender, 'authzgate-send')
            remaining, errors = sender.communicate(timeout=20)
            assert sender.returncode == 0 and errors == b'', 'authorize send child failed'
            # ok=true exactly for the legal purge, the PING and the STATUS.
            for entry, expected in zip(sent['outcomes'], expected_authz_outcomes):
                op, _, body = entry.partition(':')
                answer = json.loads(body)
                should_pass = expected.endswith('authorized_verdict') or expected.endswith('executed')
                assert answer['ok'] is should_pass, (entry, expected)
        finally:
            settle(sender)
        outcome = handshake(authz_process, 'authzgate-outcome')
        assert outcome['state'] == 'PASS', 'authorize case failed'
        assert outcome['category'] == 'authorize', outcome.get('category')
        assert outcome['outcomes'] == expected_authz_outcomes, outcome.get('outcomes')
        assert outcome['executable'] == ['PING', 'STATUS'], outcome.get('executable')
        remaining, errors = authz_process.communicate(timeout=15)
        assert authz_process.returncode == 0 and errors == b'', 'authorize child failed'
        # The worker-side invariant already proved the object is unchanged;
        # double-check from the driver so the claim does not rest on the
        # component under test alone.
        assert object_path.read_bytes() == content, 'the eligible verdict modified the object'
        record('worker_authorizes_legal_and_types_denials', True,
               'durable gate: eligible verdict for the legal envelope, typed denials '
               '(tampered/wrong_object/wrong_plan/revoked) over one real authorized connection')
    finally:
        settle(authz_process)

    print(json.dumps({'result': 'PASS' if all(r['result'] == 'PASS' for r in results) else 'FAIL',
                      'scope': 'DISPOSABLE_NATIVE_PRIVATE_WORKER_NOT_PRODUCT_OR_NAS_OR_DEPLOYMENT_ACCEPTANCE',
                      'checks': results,
                      'uids': {'worker': worker, 'caller': caller, 'outsider': outsider},
                      'fixtures_retained_until_runner_disposal': True}))
    # The proof is written first, so the artifact is complete, and only then is
    # the process failed. Printing FAIL while exiting 0 would let a step with
    # pipefail go green over a recorded failure, which is the one outcome this
    # whole probe exists to prevent.
    failed = [r['case'] for r in results if r['result'] != 'PASS']
    if failed:
        raise SystemExit('recorded failures: ' + ', '.join(failed))


if __name__ == '__main__':
    main()
