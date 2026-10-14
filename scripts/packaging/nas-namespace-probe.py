#!/usr/bin/env python3
"""OFFLINE constraint model. There is deliberately no privileged/live backend.

Witnesses below are synthetic assertions, never kernel/Docker evidence. Both
CLI modes refuse execution before opening descriptors or contacting anything.
"""
import argparse
from dataclasses import dataclass, field
import json
import re
import sys

sys.dont_write_bytecode = True
FROZEN_IMAGE = "sha256:17286831a665b3d8470db80063014cc8ab96815a12e2a1dbfa1c03f43f8eb7f0"
HEX64 = re.compile(r"[0-9a-f]{64}\Z")
NONCE = re.compile(r"[0-9a-f]{32}\Z")
ENDPOINT = ("GET", "127.0.0.1", 8080, "/healthz")
MAX_EVENTS = 9
MAX_SECONDS = 10
MAX_OUTPUT = 4096
MAX_MEMORY = 64 * 1024 * 1024
MAX_FDS = 16
PHASES = ("snapshot", "pin", "recheck", "child", "drop", "request", "response", "postcheck", "reaped")


class Refused(Exception):
    """Only internal codes may become public output; never witness repr/text."""


def require(condition, code):
    if not condition:
        raise Refused(code)


def positive(value):
    return type(value) is int and value > 0


@dataclass(frozen=True)
class Owner:
    daemon: str
    container: str
    nonce: str
    image: str
    network: str
    pid: int
    starttime: int
    boot: str
    netns: tuple
    host_netns: tuple
    running: bool = True
    pidfd_alive: bool = True
    internal: bool = True
    isolated_ipv4: bool = True
    ipv6_disabled: bool = True
    attachments: tuple = ()
    published_ports: tuple = ()


@dataclass(frozen=True)
class Tool:
    path: str
    sha256: str
    uid: int = 0
    mode: int = 0o755
    regular: bool = True
    canonical: bool = True
    trusted_ancestors: bool = True
    no_file_caps: bool = True
    pinned: bool = True


@dataclass(frozen=True)
class Handles:
    """Logical model handles, NOT real/inherited process descriptors."""
    pidfd: int
    netfd: int
    credential_fd: int
    pid: int
    starttime: int
    netns: tuple
    netns_type: str = "CLONE_NEWNET"
    private_fifo: bool = True
    exclusive_reader: bool = True
    credential_bytes: int = 64


@dataclass(frozen=True)
class Child:
    child_pidfd: int
    namespace: tuple
    only_child_switched: bool = True
    switched_types: tuple = ("CLONE_NEWNET",)
    argv: tuple = ("ndg-fixed-namespace-child",)
    environment: tuple = ()
    inherited_fds: tuple = ()


@dataclass(frozen=True)
class Drop:
    """All fields require kernel readback in a future implementation."""
    uid: tuple = (65534, 65534, 65534)
    gid: tuple = (65534, 65534, 65534)
    groups: tuple = ()
    caps: tuple = (0, 0, 0, 0, 0)  # inheritable/permitted/effective/bounding/ambient
    no_new_privs: bool = True
    readback: bool = True
    max_seconds: int = MAX_SECONDS
    max_output: int = MAX_OUTPUT
    max_memory: int = MAX_MEMORY
    max_fds: int = MAX_FDS


@dataclass(frozen=True)
class Request:
    endpoint: tuple = ENDPOINT
    dns: bool = False
    proxy: bool = False
    redirect: bool = False
    shell: bool = False
    credential_channel: str = "private_fd"
    requests: int = 1


@dataclass(frozen=True)
class Response:
    status: int = 200
    output_bytes: int = 0
    elapsed: int = 1
    secret_free: bool = True
    credential_fd_closed: bool = True


@dataclass(frozen=True)
class Result:
    status: str
    reason: str
    cleanup_intents: tuple = field(default=(), repr=False)

    def public(self):
        return {"status": self.status, "reason": self.reason, "evidence": "OFFLINE_MODEL_ONLY",
                "kernel_namespace_http": "NOT_RUN", "host_loopback_publish": "NOT_RUN",
                "live_backend": "BLOCKED"}


def valid_owner(owner):
    require(type(owner) is Owner, "invalid_owner")
    for value in (owner.daemon, owner.container, owner.network):
        require(type(value) is str and HEX64.fullmatch(value), "owner_identity_unconfirmed")
    require(type(owner.nonce) is str and NONCE.fullmatch(owner.nonce), "nonce_unconfirmed")
    require(type(owner.image) is str and owner.image == FROZEN_IMAGE, "image_unconfirmed")
    require(positive(owner.pid) and positive(owner.starttime), "process_identity_unconfirmed")
    require(type(owner.boot) is str and NONCE.fullmatch(owner.boot), "boot_identity_unconfirmed")
    for namespace in (owner.netns, owner.host_netns):
        require(type(namespace) is tuple and len(namespace) == 2 and all(positive(v) for v in namespace), "namespace_unconfirmed")
    require(owner.netns != owner.host_netns, "host_namespace_refused")
    require(all(value is True for value in (owner.running, owner.pidfd_alive, owner.internal,
                                           owner.isolated_ipv4, owner.ipv6_disabled)), "isolation_or_liveness_unconfirmed")
    require(owner.attachments == (owner.network,) and owner.published_ports == (), "network_scope_unconfirmed")


def valid_tool(tool):
    require(type(tool) is Tool and tool.path == "/usr/bin/nsenter", "trusted_tool_missing")
    require(type(tool.sha256) is str and HEX64.fullmatch(tool.sha256), "trusted_tool_missing")
    require(type(tool.uid) is int and tool.uid == 0 and type(tool.mode) is int
            and tool.mode & 0o7022 == 0 and tool.mode & 0o111 != 0, "trusted_tool_missing")
    require(all(value is True for value in (tool.regular, tool.canonical, tool.trusted_ancestors,
                                           tool.no_file_caps, tool.pinned)), "trusted_tool_missing")


def replay(expected, tool, events):
    """Pure bounded replay; emits logical cleanup intents but executes none.

    No callback/adapter/command/URL/path arguments are accepted. A successful
    replay means only that synthetic assertions met the proposed constraints.
    The CLI never accepts witnesses, and a live backend cannot be injected.
    """
    owned = []
    child = None
    reaped = False
    reason = "model_constraints_satisfied"
    status = "OFFLINE_MODEL_ACCEPTED"
    try:
        valid_owner(expected)
        valid_tool(tool)
        require(type(events) is tuple and len(events) <= MAX_EVENTS, "event_budget_exceeded")
        handles = None
        for phase, event in zip(PHASES, events):
            require(type(event) is tuple and len(event) == 2 and event[0] == phase, "phase_order_refused")
            value = event[1]
            if phase in {"snapshot", "recheck", "postcheck"}:
                require(type(value) is tuple and len(value) == 2, "snapshot_unconfirmed")
                owner, observed_tool = value
                valid_owner(owner)
                valid_tool(observed_tool)
                require(owner == expected, "owner_changed")
                require(observed_tool == tool, "tool_changed")
            elif phase == "pin":
                require(type(value) is Handles, "handles_unconfirmed")
                # Model each valid exclusive handle as acquired even if a later
                # ownership check fails, so refusal retains a cleanup obligation.
                for fd in (value.pidfd, value.netfd, value.credential_fd):
                    require(type(fd) is int and fd >= 3 and fd not in owned, "handles_unconfirmed")
                    owned.append(fd)
                handles = value
                require(positive(value.pid) and positive(value.starttime) and type(value.netns) is tuple
                        and len(value.netns) == 2 and all(positive(v) for v in value.netns), "pinned_process_changed")
                require((value.pid, value.starttime, value.netns) == (expected.pid, expected.starttime, expected.netns), "pinned_process_changed")
                require(value.netns_type == "CLONE_NEWNET", "namespace_type_refused")
                require(value.private_fifo is True and value.exclusive_reader is True
                        and positive(value.credential_bytes) and value.credential_bytes <= 512, "credential_channel_refused")
            elif phase == "child":
                require(type(value) is Child, "child_unconfirmed")
                require(type(value.child_pidfd) is int and value.child_pidfd >= 3
                        and value.child_pidfd not in owned, "child_handle_unconfirmed")
                child = value.child_pidfd
                owned.append(child)
                require(type(value.namespace) is tuple and len(value.namespace) == 2
                        and all(positive(v) for v in value.namespace), "child_namespace_refused")
                require(value.namespace == expected.netns and value.only_child_switched is True
                        and value.switched_types == ("CLONE_NEWNET",), "child_namespace_refused")
                require(value.argv == ("ndg-fixed-namespace-child",) and value.environment == (), "command_or_environment_refused")
                require(type(value.inherited_fds) is tuple and all(type(fd) is int for fd in value.inherited_fds), "fd_inheritance_refused")
                require(value.inherited_fds == (handles.netfd, handles.credential_fd), "fd_inheritance_refused")
            elif phase == "drop":
                require(type(value) is Drop and value == Drop(), "privilege_or_limits_unconfirmed")
                require(all(type(items) is tuple and all(type(v) is int for v in items)
                            for items in (value.uid, value.gid, value.groups, value.caps)), "privilege_or_limits_unconfirmed")
                require(all(type(v) is int for v in (value.max_seconds, value.max_output, value.max_memory, value.max_fds)), "privilege_or_limits_unconfirmed")
                require(value.readback is True and value.no_new_privs is True, "privilege_or_limits_unconfirmed")
            elif phase == "request":
                require(type(value) is Request and value == Request(), "request_scope_refused")
                require(type(value.requests) is int and type(value.endpoint) is tuple
                        and type(value.endpoint[2]) is int, "request_scope_refused")
                require(all(v is False for v in (value.dns, value.proxy, value.redirect, value.shell)), "request_scope_refused")
            elif phase == "response":
                require(type(value) is Response, "response_unconfirmed")
                require(type(value.status) is int and value.status == 200, "http_status_refused")
                require(type(value.output_bytes) is int and 0 <= value.output_bytes <= MAX_OUTPUT, "output_budget_exceeded")
                require(positive(value.elapsed) and value.elapsed <= MAX_SECONDS, "timeout")
                require(value.secret_free is True and value.credential_fd_closed is True, "private_channel_unconfirmed")
            elif phase == "reaped":
                require(value is True, "child_reap_unconfirmed")
                reaped = True
        require(len(events) == len(PHASES), "incomplete_protocol")
    except Refused as error:
        status, reason = "BLOCKED", str(error)
    except Exception:
        # Caller payloads, reprs and exception messages never become logs.
        status, reason = "BLOCKED", "malformed_model_input"
    cleanup = []
    if child is not None and not reaped:
        cleanup.extend((("kill_via_owned_pidfd", child), ("wait_via_owned_pidfd", child)))
    cleanup.extend(("close_owned_model_fd", fd) for fd in reversed(owned))
    return Result(status, reason, tuple(cleanup))


class SafeParser(argparse.ArgumentParser):
    def error(self, message):
        # argparse's usual error includes rejected argument text. Discard it.
        self.exit(2, json.dumps(Result("BLOCKED", "unsupported_arguments").public(), sort_keys=True) + "\n")


def main(argv=None):
    parser = SafeParser(description=__doc__)
    parser.add_argument("--prototype", action="store_true", help="show the blocked prototype gate; performs no I/O")
    args = parser.parse_args(argv)
    result = Result("BLOCKED", "live_backend_requires_independent_review" if args.prototype else "protocol_only_no_io")
    print(json.dumps(result.public(), sort_keys=True))
    return 2 if args.prototype else 0


if __name__ == "__main__":
    sys.exit(main())
