#!/usr/bin/env python3
"""Synthetic offline tests. Docker is replaced by a strict, disposable mock."""
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parent
SOURCE = SCRIPTS.parents[1] / "packaging/fnos/ndg-readonly-test"
spec = importlib.util.spec_from_file_location("fnos_check", SCRIPTS / "check-fnos-package.py")
sys.dont_write_bytecode = True
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


class PackageChecks(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ndg-fnos-offline-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "package"
        shutil.copytree(SOURCE, self.root)

    def compose(self):
        return json.loads((self.root / "app/docker/docker-compose.yaml").read_text())

    def write_compose(self, value):
        (self.root / "app/docker/docker-compose.yaml").write_text(json.dumps(value))

    def reject_compose(self, modify, code):
        value = self.compose()
        modify(value["services"]["ndg"])
        self.write_compose(value)
        with self.assertRaisesRegex(check.Rejected, "^" + code + "$"):
            check.validate(self.root)

    def test_research_only_structure(self):
        self.assertEqual(check.validate(self.root), "research_structure_pass_not_installable")

    def test_dangerous_fields_rejected(self):
        for key, value in [("privileged", True), ("network_mode", "host"), ("devices", ["/dev:/dev"]), ("cap_add", ["SYS_ADMIN"]), ("entrypoint", "sh"), ("command", "unreviewed"), ("env_file", "credentials")]:
            with self.subTest(key=key):
                self.reject_compose(lambda s: s.update({key: value}), "unreviewed_compose_field")
                self.write_compose(json.loads((SOURCE / "app/docker/docker-compose.yaml").read_text()))

    def test_root_user_rejected(self):
        self.reject_compose(lambda s: s.update(user="0:0"), "unsafe_container_privilege")

    def test_rootfs_write_rejected(self):
        self.reject_compose(lambda s: s.update(read_only=False), "unsafe_container_privilege")

    def test_public_listener_rejected(self):
        self.reject_compose(lambda s: s.update(ports=["18080:8080"]), "public_or_unreviewed_listener")

    def test_source_write_rejected(self):
        self.reject_compose(lambda s: s["volumes"][1].update(read_only=False), "unsafe_volumes")

    def test_socket_mount_rejected(self):
        self.reject_compose(lambda s: s["volumes"].append({"type": "bind", "source": "/var/run/docker.sock", "target": "/var/run/docker.sock"}), "unsafe_volumes")

    def test_undeclared_credentials_rejected(self):
        self.reject_compose(lambda s: s["environment"].update(PASSWORD="synthetic-marker"), "unsafe_environment")

    def test_no_image_tag_drift(self):
        self.reject_compose(lambda s: s.update(image="example/ndg:latest"), "unfrozen_image")

    def test_no_project_data_share(self):
        file = self.root / "config/resource"
        value = json.loads(file.read_text())
        value["data-share"] = {"shares": [{"name": "internal-db"}]}
        file.write_text(json.dumps(value))
        with self.assertRaisesRegex(check.Rejected, "unsafe_resources"):
            check.validate(self.root)

    def test_no_symlink_or_generated_entries(self):
        link = self.root / "escape"
        link.symlink_to(Path(self.temp.name))
        with self.assertRaisesRegex(check.Rejected, "package_symlink"):
            check.validate(self.root)
        link.unlink()
        (self.root / ".DS_Store").write_text("synthetic")
        with self.assertRaisesRegex(check.Rejected, "hidden_package_entry"):
            check.validate(self.root)

    def test_duplicate_json_keys_rejected(self):
        (self.root / "config/resource").write_text('{"docker-project": {}, "docker-project": {}}')
        with self.assertRaisesRegex(check.Rejected, "duplicate_json_key"):
            check.validate(self.root)

    def test_missing_receipt_digest_and_forged_pass_cannot_build(self):
        with self.assertRaisesRegex(check.Rejected, "accepted_oci_digest_required"):
            check.validate(self.root, release=True)
        # Synthetic digest used only to test rejection; never a published identity.
        image = "invalid.example/synthetic@sha256:" + "a" * 64
        value = self.compose()
        value["services"]["ndg"]["image"] = image
        self.write_compose(value)
        with self.assertRaisesRegex(check.Rejected, "acceptance_receipt_required"):
            check.validate(self.root, release=True)
        receipt = {"schema": 1, "source_sha": "b" * 40, "image_digest": image, "docker_acceptance": "PASS", "fnos_acceptance": "PASS", "architectures": ["linux/amd64"]}
        with self.assertRaisesRegex(check.Rejected, "fnos_runtime_integration_not_implemented"):
            check.validate(self.root, release=True, receipt=receipt)
        receipt["docker_acceptance"] = "NOT_RUN"
        with self.assertRaisesRegex(check.Rejected, "acceptance_not_complete"):
            check.validate(self.root, release=True, receipt=receipt)

    def test_wrapper_never_produces_fpk(self):
        result = subprocess.run(["sh", str(SCRIPTS / "build-fnos-package.sh"), "--root", str(self.root)], capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 1)
        self.assertIn("accepted_oci_digest_required", result.stderr)
        self.assertEqual(list(self.root.rglob("*.fpk")), [])

    def test_malformed_input_and_errors_are_sanitized(self):
        (self.root / "config/resource").write_text("synthetic_private_marker malformed json")
        result = subprocess.run(["python3", str(SCRIPTS / "check-fnos-package.py"), "--root", str(self.root)], capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 1)
        self.assertNotIn("synthetic_private_marker", result.stdout + result.stderr)
        self.assertNotIn(str(self.root), result.stdout + result.stderr)


class LifecycleChecks(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ndg-fnos-lifecycle-")
        self.addCleanup(self.temp.cleanup)
        self.private = Path(self.temp.name)
        self.mock = self.private / "mock"
        self.mock.mkdir()
        docker = self.mock / "docker"
        docker.write_text('''#!/bin/sh
set -eu
[ "$#" = 4 ] && [ "$1" = inspect ] && [ "$2" = --format ] && [ "$4" = ndg-readonly-test-web ] || exit 99
printf '%s' "$3" | grep -q 'com.ndg.fnos.app' || exit 98
printf '%s\\n' "${MOCK_INSPECT:-}"
printf '%s\\n' 'synthetic_private_daemon_error' >&2
exit "${MOCK_EXIT:-0}"
''')
        docker.chmod(0o700)
        self.env = {"PATH": str(self.mock) + ":/usr/bin:/bin", "MOCK_INSPECT": "ndg-readonly-test|fnos-research|true|healthy", "MOCK_EXIT": "0"}

    def run_main(self, action):
        result = subprocess.run(["sh", str(SOURCE / "cmd/main"), action], env=self.env, capture_output=True, text=True, timeout=3)
        self.assertNotIn("synthetic_private_daemon_error", result.stdout + result.stderr)
        return result

    def test_status_healthy_exact_identity_only(self):
        self.assertEqual(self.run_main("status").returncode, 0)
        for value in ["", "other-app|fnos-research|true|healthy", "ndg-readonly-test|other-channel|true|healthy", "ndg-readonly-test|fnos-research|false|healthy", "ndg-readonly-test|fnos-research|true|unhealthy", "ndg-readonly-test|fnos-research|true|missing", "ndg-readonly-test|fnos-research|true|starting"]:
            with self.subTest(value=value):
                self.env["MOCK_INSPECT"] = value
                self.assertEqual(self.run_main("status").returncode, 3)

    def test_daemon_failure_is_not_running(self):
        self.env["MOCK_EXIT"] = "1"
        self.assertEqual(self.run_main("status").returncode, 3)

    def test_unsupported_lifecycle_no_mutation(self):
        marker = self.private / "synthetic-project-data"
        marker.write_bytes(b"immutable synthetic source and db substitute")
        before = hashlib.sha256(marker.read_bytes()).hexdigest()
        for action in ["start", "stop", "unexpected"]:
            self.assertEqual(self.run_main(action).returncode, 1)
        for name in ["install_init", "install_callback", "upgrade_init", "upgrade_callback", "config_init", "config_callback"]:
            result = subprocess.run(["sh", str(SOURCE / "cmd" / name)], env=self.env, capture_output=True, text=True, timeout=3)
            self.assertEqual(result.returncode, 1)
            self.assertIn("unsupported_lifecycle_operation", result.stderr)
        for name in ["uninstall_init", "uninstall_callback"]:
            result = subprocess.run(["sh", str(SOURCE / "cmd" / name)], env=self.env, capture_output=True, text=True, timeout=3)
            self.assertEqual(result.returncode, 0)
        self.assertEqual(hashlib.sha256(marker.read_bytes()).hexdigest(), before)


if __name__ == "__main__":
    unittest.main()
