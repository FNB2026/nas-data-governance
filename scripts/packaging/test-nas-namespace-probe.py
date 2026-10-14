#!/usr/bin/env python3
"""Pure mocked protocol and executable refusal tests. NO kernel/netns/Docker tests."""
import contextlib
from dataclasses import replace
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import unittest
from unittest import mock

sys.dont_write_bytecode = True
script = Path(__file__).with_name("nas-namespace-probe.py")
spec = importlib.util.spec_from_file_location("namespace_probe", script)
probe = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = probe
spec.loader.exec_module(probe)


def fixture():
    owner = probe.Owner("a" * 64, "b" * 64, "c" * 32, probe.FROZEN_IMAGE,
                        "d" * 64, 123, 456, "e" * 32, (4, 5), (4, 6), attachments=("d" * 64,))
    tool = probe.Tool("/usr/bin/nsenter", "f" * 64)
    handles = probe.Handles(3, 4, 5, owner.pid, owner.starttime, owner.netns)
    child = probe.Child(6, owner.netns, inherited_fds=(4, 5))
    values = ((owner, tool), handles, (owner, tool), child, probe.Drop(), probe.Request(),
              probe.Response(), (owner, tool), True)
    return owner, tool, tuple(zip(probe.PHASES, values))


def changed(events, index, value):
    return events[:index] + ((events[index][0], value),) + events[index + 1:]


class ModelTests(unittest.TestCase):
    def setUp(self):
        self.owner, self.tool, self.events = fixture()

    def replay(self, events=None):
        return probe.replay(self.owner, self.tool, self.events if events is None else events)

    def assert_blocked(self, events, code):
        result = self.replay(events)
        self.assertEqual((result.status, result.reason), ("BLOCKED", code))
        return result

    def test_valid_replay_is_only_offline_model_and_closes_all_handles(self):
        result = self.replay()
        self.assertEqual(result.status, "OFFLINE_MODEL_ACCEPTED")
        self.assertEqual(result.cleanup_intents, tuple(("close_owned_model_fd", fd) for fd in (6, 5, 4, 3)))
        public = result.public()
        self.assertEqual(public["kernel_namespace_http"], "NOT_RUN")
        self.assertEqual(public["host_loopback_publish"], "NOT_RUN")
        self.assertEqual(public["live_backend"], "BLOCKED")
        self.assertNotIn("PASS", json.dumps(public))

    def test_wrong_pid_pid_reuse_and_namespace_handle_refused(self):
        for update in ({"pid": 999}, {"starttime": 999}, {"netns": (4, 999)},
                       {"pid": 123.0}, {"starttime": 456.0}, {"netns": (4.0, 5.0)}):
            with self.subTest(update=update):
                result = self.assert_blocked(changed(self.events, 1, replace(self.events[1][1], **update)), "pinned_process_changed")
                self.assertEqual(len(result.cleanup_intents), 3)

    def test_owner_and_tool_rechecked_before_and_after_http(self):
        for index in (0, 2, 7):
            for update in ({"pid": 999}, {"starttime": 999}, {"netns": (4, 999)},
                           {"container": "9" * 64}, {"nonce": "9" * 32},
                           {"daemon": "9" * 64}, {"boot": "9" * 32},
                           {"host_netns": (4, 999)}):
                with self.subTest(index=index, update=update):
                    self.assert_blocked(changed(self.events, index, (replace(self.owner, **update), self.tool)), "owner_changed")
            self.assert_blocked(changed(self.events, index, (self.owner, replace(self.tool, sha256="9" * 64))), "tool_changed")

    def test_frozen_image_exact_attachment_isolation_and_liveness(self):
        cases = (({"image": "sha256:" + "9" * 64}, "image_unconfirmed"),
                 ({"attachments": (self.owner.network, "9" * 64)}, "network_scope_unconfirmed"),
                 ({"published_ports": (8080,)}, "network_scope_unconfirmed"),
                 ({"netns": self.owner.host_netns}, "host_namespace_refused"))
        for key in ("internal", "isolated_ipv4", "ipv6_disabled", "running", "pidfd_alive"):
            cases += (({key: False}, "isolation_or_liveness_unconfirmed"),)
        for update, code in cases:
            with self.subTest(update=update):
                self.assert_blocked(changed(self.events, 2, (replace(self.owner, **update), self.tool)), code)

    def test_missing_untrusted_or_mutable_tool(self):
        cases = ({"path": "/tmp/nsenter"}, {"uid": 1}, {"mode": 0o777}, {"mode": 0o4755},
                 {"canonical": False}, {"trusted_ancestors": False}, {"regular": False},
                 {"no_file_caps": False}, {"pinned": False}, {"sha256": ""})
        for update in cases:
            with self.subTest(update=update):
                result = probe.replay(self.owner, replace(self.tool, **update), self.events)
                self.assertEqual(result.reason, "trusted_tool_missing")
                self.assertEqual(result.cleanup_intents, ())

    def test_no_arbitrary_namespace_path_or_master_switch(self):
        self.assert_blocked(changed(self.events, 1, replace(self.events[1][1], netns_type="CLONE_NEWUSER")), "namespace_type_refused")
        for update in ({"namespace": self.owner.host_netns}, {"only_child_switched": False},
                       {"switched_types": ("CLONE_NEWNET", "CLONE_NEWNS")}):
            with self.subTest(update=update):
                self.assert_blocked(changed(self.events, 3, replace(self.events[3][1], **update)), "child_namespace_refused")

    def test_privilege_drop_readback_and_all_limits_mandatory(self):
        cases = ({"uid": (0, 0, 0)}, {"gid": (0, 0, 0)}, {"groups": (0,)},
                 {"caps": (0, 0, 0, 1, 0)}, {"no_new_privs": False}, {"readback": False},
                 {"uid": (65534.0,) * 3}, {"max_seconds": 11}, {"max_memory": 0},
                 {"max_output": 8192}, {"max_fds": 1024})
        for update in cases:
            with self.subTest(update=update):
                result = self.assert_blocked(changed(self.events, 4, replace(probe.Drop(), **update)), "privilege_or_limits_unconfirmed")
                self.assertEqual(result.cleanup_intents[:2], (("kill_via_owned_pidfd", 6), ("wait_via_owned_pidfd", 6)))

    def test_url_dns_proxy_redirect_and_command_injection_refused(self):
        for endpoint in (("GET", "localhost", 8080, "/healthz"),
                         ("GET", "127.0.0.1.evil", 8080, "/healthz"),
                         ("GET", "127.0.0.1", 8080, "//evil"),
                         ("GET", "127.0.0.1", 80, "/healthz"),
                         ("POST", "127.0.0.1", 8080, "/api/scan")):
            self.assert_blocked(changed(self.events, 5, replace(probe.Request(), endpoint=endpoint)), "request_scope_refused")
        for update in ({"dns": True}, {"proxy": True}, {"redirect": True}, {"shell": True},
                       {"requests": 2}, {"requests": True}, {"credential_channel": "argv"}):
            self.assert_blocked(changed(self.events, 5, replace(probe.Request(), **update)), "request_scope_refused")
        for update in ({"argv": ("sh", "-c", "injected")}, {"environment": (("HTTP_PROXY", "injected"),)}):
            self.assert_blocked(changed(self.events, 3, replace(self.events[3][1], **update)), "command_or_environment_refused")

    def test_private_fd_exclusive_bounded_no_extra_inheritance(self):
        for update in ({"private_fifo": False}, {"exclusive_reader": False}, {"credential_bytes": 513}):
            self.assert_blocked(changed(self.events, 1, replace(self.events[1][1], **update)), "credential_channel_refused")
        for update in ({"credential_fd": 1}, {"credential_fd": 4}, {"pidfd": True}):
            self.assert_blocked(changed(self.events, 1, replace(self.events[1][1], **update)), "handles_unconfirmed")
        self.assert_blocked(changed(self.events, 3, replace(self.events[3][1], inherited_fds=(3, 4, 5))), "fd_inheritance_refused")

    def test_timeout_redirect_output_secret_and_pipe_close_failures(self):
        for update, code in (({"elapsed": 11}, "timeout"), ({"status": 302}, "http_status_refused"),
                             ({"output_bytes": 4097}, "output_budget_exceeded"),
                             ({"secret_free": False}, "private_channel_unconfirmed"),
                             ({"credential_fd_closed": False}, "private_channel_unconfirmed")):
            result = self.assert_blocked(changed(self.events, 6, replace(probe.Response(), **update)), code)
            self.assertEqual(result.cleanup_intents[:2], (("kill_via_owned_pidfd", 6), ("wait_via_owned_pidfd", 6)))
            self.assertEqual(result.cleanup_intents[2:], tuple(("close_owned_model_fd", fd) for fd in (6, 5, 4, 3)))

    def test_partial_protocol_retains_cleanup_obligations_and_never_deletes(self):
        for index in range(9):
            result = self.assert_blocked(self.events[:index], "incomplete_protocol")
            self.assertFalse(any("delete" in action or "remove" in action for action, _ in result.cleanup_intents))
            if index >= 4:
                self.assertEqual(result.cleanup_intents[:2], (("kill_via_owned_pidfd", 6), ("wait_via_owned_pidfd", 6)))
        self.assert_blocked(self.events + (("extra", True),), "event_budget_exceeded")
        self.assert_blocked(self.events[:4] + (self.events[5],) + self.events[4:5] + self.events[6:], "phase_order_refused")
        self.assert_blocked(changed(self.events, 8, False), "child_reap_unconfirmed")

    def test_public_output_never_contains_input_or_exception_text(self):
        secret = "synthetic_private_secret_not_for_output"
        result = self.replay(changed(self.events, 3, replace(self.events[3][1], environment=(("SECRET", secret),))))
        self.assertNotIn(secret, json.dumps(result.public()))
        class Bad:
            def __eq__(self, other):
                raise RuntimeError(secret)
        result = self.replay(changed(self.events, 5, replace(probe.Request(), endpoint=Bad())))
        self.assertEqual(result.reason, "malformed_model_input")
        self.assertNotIn(secret, json.dumps(result.public()))
        self.assertNotIn(self.owner.container, json.dumps(result.public()))
        self.assertNotIn(self.owner.nonce, json.dumps(result.public()))


class RefusalTests(unittest.TestCase):
    def test_default_and_explicit_modes_have_no_effect_api_calls(self):
        for args, exit_code, reason in (([], 0, "protocol_only_no_io"),
                                        (["--prototype"], 2, "live_backend_requires_independent_review")):
            with self.subTest(args=args):
                output = io.StringIO()
                with mock.patch("builtins.open", side_effect=AssertionError("file I/O")), \
                     mock.patch("subprocess.run", side_effect=AssertionError("process I/O")), \
                     mock.patch("socket.socket", side_effect=AssertionError("network I/O")), \
                     mock.patch("os.open", side_effect=AssertionError("descriptor I/O")), \
                     contextlib.redirect_stdout(output):
                    self.assertEqual(probe.main(args), exit_code)
                self.assertEqual(json.loads(output.getvalue())["reason"], reason)

    def test_actual_cli_refuses_execution_and_unknown_command_input(self):
        # Executable CLI contract only; this is not an actual kernel test.
        for args, code in (([], 0), (["--prototype"], 2), (["--pid", "123"], 2),
                           (["--url", "http://127.0.0.1:8080"], 2), (["--command", "sh"], 2),
                           (["--credential", "synthetic_secret_not_for_cli_logs"], 2)):
            child = subprocess.run([sys.executable, "-B", str(script), *args], capture_output=True, text=True, timeout=5)
            self.assertEqual(child.returncode, code)
            self.assertNotIn("synthetic_secret_not_for_cli_logs", child.stdout + child.stderr)
            if args in ([], ["--prototype"]):
                self.assertEqual(json.loads(child.stdout)["live_backend"], "BLOCKED")


if __name__ == "__main__":
    unittest.main(verbosity=2)
