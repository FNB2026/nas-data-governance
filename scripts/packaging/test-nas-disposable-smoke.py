#!/usr/bin/env python3
"""Offline synthetic bytes and mocked Docker only; not NAS runtime evidence."""
import contextlib
import copy
import hashlib
import importlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import stat
import tarfile
import tempfile
import unittest
import urllib.error
from types import SimpleNamespace
from unittest import mock

sys.dont_write_bytecode = True
smoke = importlib.import_module("nas-disposable-smoke")
fixtures = importlib.import_module("test-container-evidence")


class FakeDocker:
    def __init__(self, nonce, image):
        self.calls = []
        self.network = "1" * 64
        self.container = "2" * 64
        self.values = {
            ("network", self.network): [{"Id": self.network, "Labels": {smoke.LABEL: nonce}, "Internal": True, "Driver": "bridge", "Containers": {}}],
            ("container", self.container): [{"Id": self.container, "Config": {"Labels": {smoke.LABEL: nonce}}, "Image": image}],
            ("image", image): [{"Id": image, "RepoTags": ["ndg-smoke-local:" + nonce]}],
        }

    def value(self, args):
        self.calls.append(args)
        return copy.deepcopy(self.values[(args[0], args[2])])

    def call(self, args, **kwargs):
        self.calls.append(args)
        output = self.network if args[:2] == ["network", "create"] else self.container if args[:2] == ["container", "create"] else ""
        return subprocess.CompletedProcess(args, 0, output.encode(), b"")


class SmokeChecks(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ndg-smoke-offline-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        (self.root / "private").mkdir(mode=0o700)
        self.nonce = "a" * 32
        self.image = "sha256:" + "b" * 64
        self.docker = FakeDocker(self.nonce, self.image)
        self.resources = smoke.Resources(self.docker, self.nonce, self.image, self.root)
        container = self.docker.values[("container", self.docker.container)][0]
        container["HostConfig"] = {"NetworkMode": self.docker.network, "ReadonlyRootfs": True, "Privileged": False, "Memory": 384 * 1024 * 1024, "NanoCpus": 1_000_000_000, "PidsLimit": 128, "CapDrop": ["ALL"]}
        container["Config"]["User"] = "10001:10001"
        container["HostConfig"].update(SecurityOpt=["no-new-privileges"], Tmpfs={"/tmp": "rw,nosuid,nodev,noexec,size=64m"})
        container["Mounts"] = [{"Type": "bind", "Source": str(self.root / name), "Destination": target, "RW": rw} for name, target, rw in [("source", "/sources/source_a", False), ("config", "/config", False), ("state", "/data", True)]]

    def synthetic_bundle(self):
        case = fixtures.EvidenceChecks()
        case.setUp()
        self.addCleanup(case.doCleanups)
        case.bundle()
        identity = case.receipt["image"]
        expected = {"commit": fixtures.SOURCE["commit"], "tree": fixtures.SOURCE["tree"], "archive_sha256": identity["archive_sha256"], "config_digest": identity["config_digest"]}
        return case, expected

    def test_default_protocol_never_calls_docker_or_creates_files(self):
        with mock.patch.object(smoke.subprocess, "run", side_effect=AssertionError("Docker forbidden")), mock.patch.object(smoke.tempfile, "mkdtemp", side_effect=AssertionError("write forbidden")), contextlib.redirect_stdout(io.StringIO()) as output:
            self.assertEqual(smoke.main(["--bundle", "/synthetic-does-not-exist"]), 0)
        self.assertEqual(output.getvalue(), "nas-smoke: protocol_only_no_docker_no_files_no_credentials\n")

    def test_operator_pipe_only_accepts_inherited_writable_fifo(self):
        read, write = os.pipe()
        self.addCleanup(os.close, read)
        self.addCleanup(os.close, write)
        self.assertEqual(smoke.operator_pipe(write), write)
        for fd in [0, 1, 2, read, 999999]:
            with self.subTest(fd=fd), self.assertRaisesRegex(smoke.Blocked, "invalid_operator_pipe"):
                smoke.operator_pipe(fd)
        with (self.root / "regular").open("wb") as regular:
            with self.assertRaisesRegex(smoke.Blocked, "invalid_operator_pipe"):
                smoke.operator_pipe(regular.fileno())
        with mock.patch.object(smoke.os, "write", side_effect=AssertionError("default must ignore fd")), contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(smoke.main(["--operator-receipt-fd", "999999"]), 0)

    def test_operator_handoff_atomic_stable_private_identity(self):
        read, write = os.pipe()
        self.addCleanup(os.close, read)
        self.addCleanup(os.close, write)
        original = os.write
        with mock.patch.object(smoke.os, "write", wraps=original) as calls:
            identity = smoke.operator_handoff(write, "initial", self.root, self.nonce)
            result = {"status": "PASS", "test_status": "PASS"}
            smoke.operator_handoff(write, "final", self.root, self.nonce, result=result, receipt_sha="c" * 64, identity=identity)
            self.assertEqual(calls.call_count, 2)
            self.assertTrue(all(len(call.args[1]) <= 4096 for call in calls.call_args_list))
        initial, final = [json.loads(line) for line in os.read(read, 8192).splitlines()]
        self.assertEqual(initial["phase"], "initial")
        self.assertEqual(final["phase"], "final")
        self.assertEqual(initial["pid"], os.getpid())
        for key in ["pid", "root", "device", "inode", "nonce"]:
            self.assertEqual(initial[key], final[key])
        self.assertEqual(final["receipt_sha256"], "c" * 64)
        with self.assertRaisesRegex(smoke.Blocked, "handoff_fixture_identity_changed"):
            smoke.operator_handoff(write, "final", self.root, self.nonce, result=result, identity=dict(identity, inode=-1))

    def test_operator_preflight_rejection_has_no_fixture_or_private_stdout(self):
        read, write = os.pipe()
        self.addCleanup(os.close, read)
        self.addCleanup(os.close, write)
        with mock.patch.object(smoke.platform, "system", return_value="Synthetic"), mock.patch.object(smoke.tempfile, "mkdtemp", side_effect=AssertionError("fixture forbidden")), mock.patch.object(smoke.subprocess, "run", side_effect=AssertionError("daemon forbidden")), contextlib.redirect_stdout(io.StringIO()) as output:
            self.assertEqual(smoke.main(["--execute-disposable", "--operator-receipt-fd", str(write)]), 1)
        final = json.loads(os.read(read, 4096))
        self.assertEqual(final["phase"], "final")
        self.assertIsNone(final["root"])
        self.assertIsNone(final["receipt_sha256"])
        self.assertEqual(final["status"], "BLOCKED")
        self.assertNotIn(str(self.root), output.getvalue())
        with mock.patch.object(smoke.tempfile, "mkdtemp", side_effect=AssertionError("fixture forbidden")), mock.patch.object(smoke.subprocess, "run", side_effect=AssertionError("daemon forbidden")), contextlib.redirect_stdout(io.StringIO()) as output:
            self.assertEqual(smoke.main(["--execute-disposable", "--operator-receipt-fd", str(read)]), 1)
        self.assertIn("invalid_operator_pipe", output.getvalue())

    def test_operator_handoff_failure_blocks_before_any_daemon_mutation(self):
        read, write = os.pipe()
        self.addCleanup(os.close, read)
        self.addCleanup(os.close, write)
        docker = mock.Mock()
        docker.call.return_value = subprocess.CompletedProcess([], 0, b"unix:///var/run/docker.sock", b"")
        docker.inventory.return_value = {}
        with mock.patch.object(smoke.platform, "system", return_value="Linux"), mock.patch.object(smoke.platform, "machine", return_value="x86_64"), mock.patch.object(smoke.os, "geteuid", return_value=0), mock.patch.object(smoke, "temporary_parent", return_value=self.root), mock.patch.object(smoke.shutil, "disk_usage", return_value=SimpleNamespace(free=2 * 1024**3)), mock.patch.object(smoke, "Docker", return_value=docker), mock.patch.object(smoke, "daemon_capacity", return_value={}), mock.patch.object(smoke, "operator_handoff", side_effect=smoke.Blocked("operator_handoff_failed")), mock.patch.object(smoke, "docker_archive", side_effect=AssertionError("conversion after failed handoff forbidden")), mock.patch.object(smoke.Path, "read_text", return_value="MemAvailable: 2000000 kB\n"), contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(smoke.main(["--execute-disposable", "--bundle", "/synthetic", "--operator-receipt-fd", str(write)]), 1)
        self.assertTrue(all(tuple(call.args[0][:2]) not in smoke.MUTATIONS for call in docker.call.call_args_list))

    def test_frozen_conversion_preserves_config_and_uncompressed_layers(self):
        case, expected = self.synthetic_bundle()
        target = self.root / "docker.tar"
        receipt, sha = smoke.docker_archive(case.root, expected, target, "ndg-smoke-local:" + self.nonce)
        self.assertEqual(sha, hashlib.sha256(target.read_bytes()).hexdigest())
        with tarfile.open(target) as archive:
            manifest = json.load(archive.extractfile("manifest.json"))[0]
            self.assertEqual(manifest["RepoTags"], ["ndg-smoke-local:" + self.nonce])
            config = archive.extractfile(manifest["Config"]).read()
            self.assertEqual("sha256:" + hashlib.sha256(config).hexdigest(), expected["config_digest"])
            self.assertEqual(set(archive.getnames()), {"manifest.json", manifest["Config"], *manifest["Layers"]})
            for name, layer in zip(manifest["Layers"], receipt["image"]["layers"]):
                self.assertEqual("sha256:" + hashlib.sha256(archive.extractfile(name).read()).hexdigest(), layer["diff_id"])

    def test_wrong_artifact_scope_or_existing_destination_rejects_before_write(self):
        case, expected = self.synthetic_bundle()
        target = self.root / "docker.tar"
        for update in [{"commit": "c" * 40}, {"tree": "c" * 40}, {"archive_sha256": "c" * 64}, {"config_digest": "sha256:" + "c" * 64}]:
            with self.subTest(update=update), self.assertRaises(Exception):
                smoke.docker_archive(case.root, dict(expected, **update), target, "ndg-smoke-local:" + self.nonce)
            self.assertFalse(target.exists())
        target.write_bytes(b"independent existing bytes")
        with self.assertRaises(FileExistsError):
            smoke.docker_archive(case.root, expected, target, "ndg-smoke-local:" + self.nonce)
        self.assertEqual(target.read_bytes(), b"independent existing bytes")

    def test_archive_symlink_and_unreviewed_payload_rejected(self):
        case, expected = self.synthetic_bundle()
        case.archive.unlink()
        case.archive.symlink_to(self.root / "outside")
        with self.assertRaises(Exception):
            smoke.docker_archive(case.root, expected, self.root / "target", "ndg-smoke-local:" + self.nonce)
        self.assertFalse((self.root / "target").exists())

    def test_no_existing_tag_or_invalid_nonce_reference(self):
        case, expected = self.synthetic_bundle()
        with self.assertRaisesRegex(smoke.Blocked, "invalid_private_tag"):
            smoke.docker_archive(case.root, expected, self.root / "target", "ndg-disposable:test")
        self.assertFalse((self.root / "target").exists())

    def test_fixture_is_seven_disposable_files_and_private_credentials(self):
        settings, password, marker = smoke.fixture(self.root, 12345)
        before = smoke.source_hashes(self.root)
        self.assertEqual(len(before), 7)
        self.assertEqual(len(set(before.values())), 4)
        self.assertEqual((self.root / "config" / "credential").read_text(), password)
        self.assertEqual((self.root / "config" / "credential").stat().st_mode & 0o777, 0o600)
        self.assertEqual(settings["origin"], "http://127.0.0.1:12345")
        self.assertEqual(before, smoke.source_hashes(self.root))
        self.assertTrue(all(marker in name for name in before))

    def test_source_symlink_and_membership_changes_are_rejected(self):
        smoke.fixture(self.root, 12345)
        member = next((self.root / "source").iterdir())
        member.unlink()
        member.symlink_to(self.root / "config" / "credential")
        with self.assertRaisesRegex(smoke.Blocked, "fixture_source_changed"):
            smoke.source_hashes(self.root)

    def test_known_ids_internal_network_loopback_and_limited_container_only(self):
        self.resources.create_network()
        identifier = self.resources.create_container(self.root, 12345)
        self.assertEqual(identifier, self.docker.container)
        call = next(args for args in self.docker.calls if args[:2] == ["container", "create"])
        self.assertIn("--read-only", call)
        self.assertIn("--memory=384m", call)
        self.assertIn("--cpus=1", call)
        self.assertIn("--pids-limit=128", call)
        self.assertIn("127.0.0.1:12345:8080", call)
        self.assertIn("type=bind,src=" + str(self.root / "source") + ",dst=/sources/source_a,readonly", call)
        self.assertIn("type=bind,src=" + str(self.root / "config") + ",dst=/config,readonly", call)
        self.resources.transition(identifier, "start")
        self.resources.transition(identifier, "stop")
        self.resources.cleanup()
        mutators = [a for a in self.docker.calls if a[:2] in [["container", "rm"], ["network", "rm"]]]
        self.assertEqual(mutators, [["container", "rm", "--force", self.docker.container], ["network", "rm", self.docker.network]])

    def test_foreign_fixture_and_transition_denied_without_docker_side_effect(self):
        for operation in [lambda: self.resources.create_container(self.root / "foreign"), lambda: self.resources.transition("c" * 64, "stop")]:
            with self.assertRaises(smoke.Blocked):
                operation()
        self.assertEqual(self.docker.calls, [])

    def test_ownership_change_retains_everything_and_never_prefix_cleans(self):
        self.resources.create_network()
        self.resources.create_container(self.root)
        self.docker.values[("container", self.docker.container)][0]["Config"]["Labels"][smoke.LABEL] = "foreign"
        self.docker.calls.clear()
        with self.assertRaisesRegex(smoke.Blocked, "resource_ownership_unknown"):
            self.resources.cleanup()
        self.assertFalse(any(args[:2] in [["container", "rm"], ["network", "rm"], ["image", "rm"]] for args in self.docker.calls))

    def test_foreign_network_endpoint_blocks_network_delete(self):
        self.resources.create_network()
        self.docker.values[("network", self.docker.network)][0]["Containers"] = {"foreign": {}}
        with self.assertRaisesRegex(smoke.Blocked, "network_has_foreign_endpoint"):
            self.resources.cleanup()
        self.assertNotIn(["network", "rm", self.docker.network], self.docker.calls)

    def test_existing_image_never_removed_new_nonce_tag_has_fail_closed_cleanup(self):
        self.resources.cleanup()
        self.assertEqual(self.docker.calls, [])
        self.resources.image_created = True
        self.docker.values[("image", self.image)][0]["RepoTags"].append("existing-production:keep")
        with self.assertRaisesRegex(smoke.Blocked, "image_ownership_unknown"):
            self.resources.cleanup()
        self.assertFalse(any(args[:2] == ["image", "rm"] for args in self.docker.calls))

    def test_unrelated_container_state_change_is_blocked_not_repaired(self):
        before = {"d" * 64: {"Status": "running", "StartedAt": "synthetic-before"}}
        smoke.check_existing(before, dict(before, **{"e" * 64: {"Status": "running"}}))
        for after in [{}, {"d" * 64: {"Status": "exited", "StartedAt": "synthetic-before"}}, {"d" * 64: {"Status": "running", "StartedAt": "synthetic-after"}}]:
            with self.assertRaisesRegex(smoke.Blocked, "existing_container_state_changed"):
                smoke.check_existing(before, after)

    def test_mount_and_resource_changes_refuse_before_start_or_cleanup(self):
        self.resources.create_network()
        for area, key, value in [("HostConfig", "ReadonlyRootfs", False), ("HostConfig", "Privileged", True), ("HostConfig", "NanoCpus", 2_000_000_000)]:
            with self.subTest(key=key):
                item = self.docker.values[("container", self.docker.container)][0]
                old = item[area][key]
                item[area][key] = value
                with self.assertRaisesRegex(smoke.Blocked, "container_isolation_changed"):
                    self.resources.own("container", self.docker.container)
                item[area][key] = old
        item["Mounts"][0]["RW"] = True
        with self.assertRaisesRegex(smoke.Blocked, "container_mounts_changed"):
            self.resources.own("container", self.docker.container)
        self.assertFalse(any(a[:2] == ["container", "start"] for a in self.docker.calls))

    def test_explicit_flag_without_complete_preflight_never_calls_docker(self):
        with mock.patch.object(smoke.subprocess, "run", side_effect=AssertionError("Docker forbidden")), mock.patch.object(smoke.tempfile, "mkdtemp", side_effect=AssertionError("write forbidden")), contextlib.redirect_stdout(io.StringIO()) as output:
            self.assertEqual(smoke.main(["--execute-disposable"]), 1)
        self.assertNotIn(str(self.root), output.getvalue())

    def metadata(self, *, mode=stat.S_IFDIR | 0o1777, uid=0, device=0, inode=1):
        return SimpleNamespace(st_mode=mode, st_uid=uid, st_dev=device, st_ino=inode,
                               mount_id=1, filesystem="ext4", mount_fingerprint="synthetic-mount", namespace=(0, 9))

    @contextlib.contextmanager
    def kernel_directory(self, mount, fdinfo="mnt_id:\t1\n", info=None):
        info = info or self.metadata()
        def read(path, *args, **kwargs):
            return mount if str(path) == "/proc/self/mountinfo" else fdinfo
        with contextlib.ExitStack() as stack:
            stack.enter_context(mock.patch.object(smoke.os, "O_PATH", 0x200000, create=True))
            opened = stack.enter_context(mock.patch.object(smoke.os, "open", return_value=39))
            closed = stack.enter_context(mock.patch.object(smoke.os, "close"))
            stack.enter_context(mock.patch.object(smoke.os, "fstat", return_value=info))
            stack.enter_context(mock.patch.object(smoke.os, "stat", return_value=self.metadata(inode=9)))
            stack.enter_context(mock.patch.object(smoke.evidence.boundary, "trusted_ancestry"))
            stack.enter_context(mock.patch.object(Path, "lstat", return_value=info))
            stack.enter_context(mock.patch.object(Path, "read_text", autospec=True, side_effect=read))
            yield opened, closed

    def test_btrfs_distinct_stat_device_requires_matching_kernel_mount_id(self):
        mount = "1 0 0:0 / / rw - ext4 synthetic-root rw\n753 1 0:56 /subvol /synthetic-daemon rw - btrfs synthetic-volume rw\n"
        info = self.metadata(mode=stat.S_IFDIR | 0o700, device=os.makedev(0, 57))
        with self.kernel_directory(mount, "pos:\t0\nmnt_id:\t753\n", info) as (opened, closed):
            witness = smoke.local_directory(Path("/synthetic-daemon"))
            self.assertEqual(witness.mount_id, 753)
            self.assertEqual(witness.filesystem, "btrfs")
            self.assertEqual(witness.st_dev, info.st_dev)
            opened.assert_called_once_with(Path("/synthetic-daemon"), smoke.os.O_PATH | smoke.os.O_DIRECTORY | smoke.os.O_NOFOLLOW | smoke.os.O_CLOEXEC)
            closed.assert_called_once_with(39)

    def test_fd_mount_missing_wrong_duplicate_or_unreadable_refuses_and_closes(self):
        mount = "1 0 0:0 / / rw - ext4 synthetic-root rw\n"
        for text in ["", "mnt_id:\t2\n", "mnt_id:\t1\nmnt_id:\t1\n", "mnt_id:\tunconfirmed\n", "mnt_id:\n1\n"]:
            with self.subTest(fdinfo=text), self.kernel_directory(mount, text) as (_, closed):
                with self.assertRaisesRegex(smoke.Blocked, "directory_mount_identity_changed"):
                    smoke.local_directory(Path("/tmp"))
                closed.assert_called_once_with(39)
        with self.kernel_directory(mount) as (_, closed), mock.patch.object(Path, "read_text", side_effect=OSError("synthetic private path")):
            with self.assertRaisesRegex(smoke.Blocked, "^kernel_directory_identity_unconfirmed$"):
                smoke.local_directory(Path("/tmp"))
            closed.assert_called_once_with(39)

    def test_duplicate_mount_ids_and_changed_table_refuse_and_close(self):
        mount = "1 0 0:0 / / rw - ext4 synthetic-root rw\n"
        for text in [mount + "1 0 0:0 / /unrelated rw - ext4 synthetic-other rw\n", mount + "2 1 0:0 / /tmp rw - tmpfs synthetic-a rw\n3 2 0:0 / /tmp rw - tmpfs synthetic-b rw\n"]:
            with self.subTest(table=text), self.kernel_directory(text) as (_, closed):
                with self.assertRaises(smoke.Blocked):
                    smoke.local_directory(Path("/tmp"))
                closed.assert_called_once_with(39)
        with self.kernel_directory(mount) as (_, closed), mock.patch.object(Path, "read_text", side_effect=[mount, "mnt_id:\t1\n", mount.replace("ext4", "nfs")]):
            with self.assertRaises(smoke.Blocked):
                smoke.local_directory(Path("/tmp"))
            closed.assert_called_once_with(39)

    def test_fd_and_path_identity_and_mount_namespace_are_independent(self):
        mount = "1 0 0:0 / / rw - ext4 synthetic-root rw\n"
        for changes in [[self.metadata(inode=2)], [self.metadata(), self.metadata(device=1)]]:
            with self.subTest(fd=changes), self.kernel_directory(mount) as (_, closed), mock.patch.object(smoke.os, "fstat", side_effect=changes):
                with self.assertRaisesRegex(smoke.Blocked, "directory_identity_changed"):
                    smoke.local_directory(Path("/tmp"))
                closed.assert_called_once_with(39)
        with self.kernel_directory(mount) as (_, closed), mock.patch.object(smoke.os, "stat", side_effect=[self.metadata(inode=9), self.metadata(inode=10)]):
            with self.assertRaisesRegex(smoke.Blocked, "mount_namespace_changed"):
                smoke.local_directory(Path("/tmp"))
            closed.assert_called_once_with(39)

    def test_kernel_directory_handle_missing_or_open_failure_never_falls_back(self):
        mount = "1 0 0:0 / / rw - ext4 synthetic-root rw\n"
        with self.kernel_directory(mount) as (opened, closed):
            flag = smoke.os.O_PATH
            del smoke.os.O_PATH
            try:
                with self.assertRaisesRegex(smoke.Blocked, "kernel_directory_handle_unavailable"):
                    smoke.local_directory(Path("/tmp"))
            finally:
                smoke.os.O_PATH = flag
            opened.assert_not_called()
            closed.assert_not_called()

    def test_directory_ancestry_change_refuses_while_fd_held(self):
        mount = "1 0 0:0 / / rw - ext4 synthetic-root rw\n"
        with self.kernel_directory(mount) as (_, closed), mock.patch.object(smoke.evidence.boundary, "trusted_ancestry", side_effect=[("synthetic", [("parent", 0, 1)]), ("synthetic", [("parent", 0, 2)])]):
            with self.assertRaisesRegex(smoke.Blocked, "directory_ancestry_changed"):
                smoke.local_directory(Path("/tmp"))
            closed.assert_called_once_with(39)
        with self.kernel_directory(mount) as (_, closed), mock.patch.object(smoke.os, "open", side_effect=OSError("private replaced path")):
            with self.assertRaisesRegex(smoke.Blocked, "^kernel_directory_identity_unconfirmed$"):
                smoke.local_directory(Path("/tmp"))
            closed.assert_not_called()

    def test_daemon_fingerprint_keeps_mount_namespace_and_filesystem(self):
        docker = mock.Mock()
        docker.value.return_value = {"root": str(self.root), "driver": "overlay2", "id": "synthetic-daemon"}
        info = self.metadata(mode=stat.S_IFDIR | 0o700)
        with mock.patch.object(smoke, "local_directory", return_value=info), mock.patch.object(smoke.shutil, "disk_usage", return_value=SimpleNamespace(free=2 * 1024**3)):
            before = smoke.daemon_capacity(docker)
            self.assertEqual(before["mount_id"], 1)
            self.assertEqual(before["filesystem"], "ext4")
            self.assertEqual(before["namespace"], (0, 9))
            for key, value in [("mount_id", 2), ("filesystem", "btrfs"), ("mount_fingerprint", "changed"), ("namespace", (0, 10))]:
                old = getattr(info, key)
                setattr(info, key, value)
                with self.subTest(key=key), self.assertRaisesRegex(smoke.Blocked, "daemon_identity_changed"):
                    smoke.daemon_capacity(docker, before)
                setattr(info, key, old)

    def test_fixed_tmp_ignores_caller_production_environment(self):
        info = self.metadata()
        text = "1 0 0:0 / / rw - ext4 synthetic-root rw\n"
        with self.kernel_directory(text, info=info), mock.patch.dict(os.environ, {"TMPDIR": "/synthetic-production", "TMP": "/synthetic-production", "TEMP": "/synthetic-production"}), mock.patch.object(smoke.evidence.boundary, "trusted_ancestry") as ancestry, mock.patch.object(smoke.shutil, "disk_usage", return_value=SimpleNamespace(free=2 * 1024**3)), mock.patch.object(smoke.tempfile, "mkdtemp", side_effect=AssertionError("no write")):
            self.assertEqual(smoke.temporary_parent(), Path("/tmp"))
            self.assertEqual(ancestry.call_args_list, [mock.call(Path("/tmp")), mock.call(Path("/tmp"))])

    def test_network_and_unknown_tmp_mounts_refuse_before_disk_or_creation(self):
        for kind in ["nfs", "nfs4", "cifs", "9p", "fuse.sshfs", "unknown"]:
            text = "1 0 0:0 / / rw - ext4 synthetic-root rw\n2 1 0:0 / /tmp rw - " + kind + " synthetic-remote rw\n"
            with self.subTest(kind=kind), self.kernel_directory(text), mock.patch.object(smoke.shutil, "disk_usage", side_effect=AssertionError("no remote statvfs")), mock.patch.object(smoke.tempfile, "mkdtemp", side_effect=AssertionError("no write")):
                with self.assertRaisesRegex(smoke.Blocked, "^nonlocal_or_unknown_filesystem$"):
                    smoke.temporary_parent()

    def test_tmp_owner_link_sticky_and_capacity_refusals(self):
        text = "1 0 0:0 / / rw - ext4 synthetic-root rw\n"
        for info, reason in [(self.metadata(uid=10001), "untrusted_local_directory"), (self.metadata(mode=stat.S_IFLNK | 0o777), "untrusted_local_directory"), (self.metadata(mode=stat.S_IFDIR | 0o755), "temporary_parent_not_root_sticky"), (self.metadata(), "insufficient_temporary_disk")]:
            with self.subTest(reason=reason), self.kernel_directory(text, info=info), mock.patch.object(smoke.shutil, "disk_usage", return_value=SimpleNamespace(free=1)), mock.patch.object(smoke.tempfile, "mkdtemp", side_effect=AssertionError("no write")):
                with self.assertRaisesRegex(smoke.Blocked, "^" + reason + "$"):
                    smoke.temporary_parent()

    def test_mount_device_ambiguity_and_directory_replacement_refuse(self):
        entries = [
            ("2 0 0:0 / / rw - ext4 synthetic-root rw\n", [self.metadata()], "directory_mount_identity_changed"),
            ("1 0 0:0 / / rw - ext4 synthetic-root rw\n2 1 0:0 / /tmp rw - tmpfs synthetic-a rw\n3 2 0:0 / /tmp rw - tmpfs synthetic-b rw\n", [self.metadata()], "nonlocal_or_unknown_filesystem"),
            ("1 0 0:0 / / rw - ext4 synthetic-root rw\n", [self.metadata(), self.metadata(inode=2)], "directory_identity_changed"),
        ]
        for text, infos, reason in entries:
            with self.subTest(reason=reason), self.kernel_directory(text), mock.patch.object(Path, "lstat", side_effect=infos), mock.patch.object(smoke.shutil, "disk_usage", side_effect=AssertionError("not proved")):
                with self.assertRaisesRegex(smoke.Blocked, "^" + reason + "$"):
                    smoke.temporary_parent()

    def test_daemon_capacity_is_independent_and_stable(self):
        docker = mock.Mock()
        docker.value.return_value = {"root": str(self.root), "driver": "overlay2", "id": "synthetic-daemon"}
        with mock.patch.object(smoke, "local_directory", return_value=self.metadata(mode=stat.S_IFDIR | 0o700)), mock.patch.object(smoke.shutil, "disk_usage", return_value=SimpleNamespace(free=2 * 1024**3)):
            witness = smoke.daemon_capacity(docker)
            self.assertEqual(smoke.daemon_capacity(docker, witness), witness)
            docker.value.return_value["id"] = "different-daemon"
            with self.assertRaisesRegex(smoke.Blocked, "daemon_identity_changed"):
                smoke.daemon_capacity(docker, witness)
        docker.value.return_value["id"] = "synthetic-daemon"
        with mock.patch.object(smoke, "local_directory", return_value=self.metadata(mode=stat.S_IFDIR | 0o700)), mock.patch.object(smoke.shutil, "disk_usage", return_value=SimpleNamespace(free=1)):
            with self.assertRaisesRegex(smoke.Blocked, "insufficient_daemon_disk"):
                smoke.daemon_capacity(docker)
        docker.value.return_value["driver"] = "unknown"
        with mock.patch.object(smoke, "local_directory", side_effect=AssertionError("unconfirmed driver")):
            with self.assertRaisesRegex(smoke.Blocked, "daemon_identity_unconfirmed"):
                smoke.daemon_capacity(docker)

    def test_every_daemon_mutation_requires_capacity_before_subprocess(self):
        docker = smoke.Docker(self.root / "private")
        for operation in smoke.MUTATIONS:
            with self.subTest(operation=operation), mock.patch.object(smoke.subprocess, "run", side_effect=AssertionError("mutation forbidden")):
                with self.assertRaisesRegex(smoke.Blocked, "daemon_capacity_not_proved"):
                    docker.call([*operation, "synthetic"])
                docker.capacity = {"synthetic": True}
                with mock.patch.object(smoke, "daemon_capacity", side_effect=smoke.Blocked("insufficient_daemon_disk")):
                    with self.assertRaisesRegex(smoke.Blocked, "insufficient_daemon_disk"):
                        docker.call([*operation, "synthetic"])
                docker.capacity = None

    def test_actual_user_security_and_tmpfs_must_match_creation(self):
        self.resources.create_network()
        item = self.docker.values[("container", self.docker.container)][0]
        for area, key, value in [("Config", "User", "0:0"), ("HostConfig", "SecurityOpt", []), ("HostConfig", "Tmpfs", {"/tmp": "rw,size=1g"})]:
            old = item[area][key]
            with self.subTest(key=key):
                item[area][key] = value
                with self.assertRaisesRegex(smoke.Blocked, "container_security_options_changed"):
                    self.resources.own("container", self.docker.container)
                item[area][key] = old

    def test_runtime_mock_classifies_complete_and_real_byte_change(self):
        self.runtime("PASS")

    def test_runtime_mock_refuses_dangerous_api_success(self):
        self.runtime("dangerous_api_accepted")

    def test_runtime_mock_checks_error_body_privacy(self):
        self.runtime("http_privacy_window_failed")

    def test_runtime_mock_detects_source_changes(self):
        self.runtime("source_bytes_changed")

    def test_completed_with_inconsistent_counters_is_failure(self):
        self.runtime("coverage_incomplete")

    def runtime(self, mode):
        settings, password, marker = smoke.fixture(self.root, 12345)
        own = {"State": {"Running": True, "OOMKilled": False}, "NetworkSettings": {"Ports": {"8080/tcp": [{"HostIp": "127.0.0.1", "HostPort": "12345"}]}, "Networks": {"synthetic-network": {"NetworkID": self.docker.network}}}}
        runtime = mock.Mock()
        runtime.containers = [self.docker.container]
        runtime.network = self.docker.network
        runtime.own.return_value = own
        runtime.create_container.side_effect = [self.docker.container, "3" * 64]
        runtime.docker.call.side_effect = lambda args, **kwargs: subprocess.CompletedProcess(args, 0, b"1" if args[:2] == ["container", "wait"] else b"safe synthetic", b"")
        source = fixtures.SOURCE
        status_calls = []
        requests = []
        def response(request, **kwargs):
            path = request.full_url.split("12345", 1)[1]
            requests.append(path)
            if path == "/api/login":
                value = {"csrf": "synthetic-session"}
            elif path == "/api/version":
                value = {"commit": source["commit"], "version": source["core_version"], "build_time": source["build_time"], "channel": source["channel"]}
            elif path == "/api/scan" and requests.count(path) == 1:
                raise urllib.error.HTTPError(request.full_url, 401, "synthetic", {}, io.BytesIO(b'{"error":"request_rejected"}'))
            elif any(path.startswith("/api/" + area + "/") for area in ["quarantine", "restore", "purge"]):
                if mode == "dangerous_api_accepted":
                    value = {"unexpected": "accepted"}
                else:
                    raw = json.dumps({"error": marker if mode == "http_privacy_window_failed" else "request_rejected"}).encode()
                    raise urllib.error.HTTPError(request.full_url, 403, "synthetic", {}, io.BytesIO(raw))
            elif path == "/api/status":
                status_calls.append(path)
                if mode == "source_bytes_changed" and len(status_calls) == 2:
                    next((self.root / "source").iterdir()).write_bytes(b"artificial change")
                value = {"state": "COMPLETED", "discovered": 7, "processed": 7, "scanned": 0 if mode == "coverage_incomplete" else 7, "failed": 0, "active": 7, "quick": 7, "full": 6, "groups": [{"copies": 2, "bytes": 300}] * 3, "checkpoint": "completed", "missing": 0, "unavailable": 0}
            else:
                value = {"state": "synthetic"}
            return io.BytesIO(json.dumps(value).encode())
        client = mock.Mock()
        client.open.side_effect = response
        with mock.patch.object(smoke.urllib.request, "build_opener", return_value=client), mock.patch.object(smoke.time, "sleep"):
            if mode == "PASS":
                value = smoke.exercise(runtime, self.root, 12345, settings, password, marker, source)
                self.assertTrue(value["source_sha_unchanged"])
                self.assertTrue(value["single_writer_refused"])
                self.assertEqual(len([path for path in requests if any(path.startswith("/api/" + area + "/") for area in ["quarantine", "restore", "purge"])]), 12)
            else:
                with self.assertRaisesRegex(smoke.Failed if mode == "coverage_incomplete" else smoke.Blocked, "^" + mode + "$"):
                    smoke.exercise(runtime, self.root, 12345, settings, password, marker, source)


if __name__ == "__main__":
    unittest.main()
