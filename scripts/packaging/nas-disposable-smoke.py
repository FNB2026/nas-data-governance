#!/usr/bin/env python3
"""Explicit disposable host test only; default is a side-effect-free protocol."""
import argparse
import fcntl
import gzip
import hashlib
import http.cookiejar
import importlib
import io
import json
import os
from pathlib import Path
import platform
import re
import secrets
import shutil
import socket
import stat
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.request

sys.dont_write_bytecode = True
evidence = importlib.import_module("container-evidence")
LABEL = "com.ndg.disposable-smoke"
ID = re.compile(r"^[0-9a-f]{64}$")
FROZEN = {"commit": "063d0ee980ae8115cc92c0c9df0c5c43e540f14b", "tree": "f56e54626b060bb05b0c66b21e473f754a63e201",
          "archive_sha256": "6d59909f00e540d93d5fc60a36754f08692f4dcecd251aaa5843b6d1e9999336",
          "config_digest": "sha256:17286831a665b3d8470db80063014cc8ab96815a12e2a1dbfa1c03f43f8eb7f0",
          "receipt_sha256": "63be102ba9339d3060643f60790b7cd282e95e2bce2a303e3440f3bd213f491a"}
LOCAL_FILESYSTEMS = {"tmpfs", "ext4", "xfs", "btrfs", "overlay", "zfs"}
DAEMON_INFO = ["info", "--format", '{"root":{{json .DockerRootDir}},"driver":{{json .Driver}},"id":{{json .ID}}}']
MUTATIONS = {("image", "load"), ("image", "rm"), ("network", "create"), ("network", "rm"),
             ("container", "create"), ("container", "start"), ("container", "stop"), ("container", "rm")}


class Blocked(Exception):
    pass


class Failed(Blocked):
    pass


def require(value, code):
    if not value:
        failure = {"source_bytes_changed", "dangerous_api_accepted", "unauthenticated_scan_accepted", "privacy_window_failed", "http_privacy_window_failed", "restart_state_lost", "second_writer_not_refused", "coverage_incomplete"}
        raise (Failed if code in failure else Blocked)(code)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def operator_pipe(fd):
    if fd is None:
        return None
    try:
        require(fd >= 3 and stat.S_ISFIFO(os.fstat(fd).st_mode), "invalid_operator_pipe")
        require(fcntl.fcntl(fd, fcntl.F_GETFL) & os.O_ACCMODE in {os.O_WRONLY, os.O_RDWR}, "invalid_operator_pipe")
        require(os.fpathconf(fd, "PC_PIPE_BUF") >= 512, "invalid_operator_pipe")
    except (OSError, ValueError):
        raise Blocked("invalid_operator_pipe") from None
    return fd


def operator_handoff(fd, phase, root, nonce, *, result=None, receipt_sha=None, identity=None):
    if fd is None:
        return
    operator_pipe(fd)  # The operator exclusively owns and continuously drains it.
    info = root.lstat() if root is not None else None
    require(info is None or stat.S_ISDIR(info.st_mode) and info.st_uid == os.getuid() and info.st_mode & 0o777 == 0o700, "handoff_fixture_identity_unconfirmed")
    value = {"phase": phase, "pid": os.getpid(), "root": str(root) if root is not None else None,
             "device": info.st_dev if info else None, "inode": info.st_ino if info else None, "nonce": nonce}
    common = {key: item for key, item in value.items() if key != "phase"}
    require(identity is None or common == identity, "handoff_fixture_identity_changed")
    if result is not None:
        value.update(status=result["status"], test_status=result.get("test_status", "BLOCKED"), receipt_sha256=receipt_sha)
    encoded = json.dumps(value, sort_keys=True).encode() + b"\n"
    require(len(encoded) <= min(4096, os.fpathconf(fd, "PC_PIPE_BUF")), "operator_handoff_too_large")
    try:
        require(os.write(fd, encoded) == len(encoded), "operator_handoff_failed")
    except OSError:
        raise Blocked("operator_handoff_failed") from None
    return common


def local_directory(path):
    """Metadata-only mount/ancestry check; no host directory enumeration."""
    path = Path(path)
    require(path.is_absolute(), "untrusted_local_directory")
    evidence.boundary.trusted_ancestry(path)
    before = path.lstat()
    require(stat.S_ISDIR(before.st_mode) and before.st_uid == 0, "untrusted_local_directory")
    text = Path("/proc/self/mountinfo").read_text()
    require(len(text) <= 4 * 1024 * 1024, "mount_table_unconfirmed")
    evidence.boundary.linux_mountpoints(text)  # Validate kernel escape/layout.
    covering = []
    for line in text.splitlines():
        fields = line.split()
        point = Path(re.sub(r"\\([0-7]{3})", lambda item: chr(int(item[1], 8)), fields[4]))
        if path == point or point in path.parents:
            covering.append((len(point.parts), fields[2], fields[fields.index("-") + 1]))
    require(bool(covering), "mount_table_unconfirmed")
    nearest = [item for item in covering if item[0] == max(value[0] for value in covering)]
    require(len(nearest) == 1 and nearest[0][2] in LOCAL_FILESYSTEMS, "nonlocal_or_unknown_filesystem")
    require(nearest[0][1] == str(os.major(before.st_dev)) + ":" + str(os.minor(before.st_dev)), "directory_mount_identity_changed")
    after = path.lstat()
    require((after.st_dev, after.st_ino, after.st_mode, after.st_uid) == (before.st_dev, before.st_ino, before.st_mode, before.st_uid), "directory_identity_changed")
    return before


def temporary_parent():
    # Never honor TMPDIR/TEMP/TMP, and never fall back to a caller-owned source.
    parent = Path("/tmp")
    info = local_directory(parent)
    require(info.st_uid == 0 and bool(info.st_mode & stat.S_ISVTX), "temporary_parent_not_root_sticky")
    require(shutil.disk_usage(parent).free >= 256 * 1024 * 1024, "insufficient_temporary_disk")
    return parent


def daemon_capacity(docker, expected=None):
    info = docker.value(DAEMON_INFO)
    require(isinstance(info, dict) and set(info) == {"root", "driver", "id"} and isinstance(info["root"], str) and info["root"].startswith("/") and info["driver"] == "overlay2" and isinstance(info["id"], str) and bool(info["id"]), "daemon_identity_unconfirmed")
    # Canonicalize the operator-configured path for metadata-only checking;
    # never create, change, enumerate or clean the production daemon directory.
    path = Path(info["root"]).resolve(strict=True)
    witness = local_directory(path)
    require(not witness.st_mode & 0o022, "daemon_directory_writable_by_other")
    capacity = {"root": str(path), "driver": info["driver"], "id": info["id"], "device": witness.st_dev, "inode": witness.st_ino}
    require(expected is None or capacity == expected, "daemon_identity_changed")
    require(shutil.disk_usage(path).free >= 1024 * 1024 * 1024, "insufficient_daemon_disk")
    return capacity


def private_write(path, data):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "wb") as output:
        output.write(data)
        output.flush()
        os.fsync(output.fileno())


def docker_archive(bundle, expected, destination, tag):
    """Verify then repack bytes; never extract a path from either archive."""
    require(re.fullmatch(r"ndg-smoke-local:[0-9a-f]{32}", tag), "invalid_private_tag")
    root = Path(os.path.abspath(bundle))
    evidence.boundary.trusted_ancestry(root)
    if "receipt_sha256" in expected:
        require(digest(evidence.regular_input(root / "receipt.json", 1024 * 1024).read_bytes()) == expected["receipt_sha256"], "frozen_receipt_mismatch")
    receipt = evidence.verify_bundle(root, expected_commit=expected["commit"], expected_tree=expected["tree"], expected_architecture="amd64")
    identity = receipt["image"]
    require(identity["archive_sha256"] == expected["archive_sha256"] and identity["config_digest"] == expected["config_digest"], "frozen_artifact_mismatch")
    require(all(item["status"] == "PASS" for item in receipt["scans"].values()), "source_evidence_blocked")
    archive = root / "image.oci.tar"
    fd = os.open(archive, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, "rb") as source:
        info = os.fstat(source.fileno())
        require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and info.st_uid == os.getuid() and info.st_size <= 128 * 1024 * 1024, "unsafe_frozen_archive")
        raw = source.read(128 * 1024 * 1024 + 1)
    require(digest(raw) == expected["archive_sha256"], "archive_changed_after_verification")
    # The immutable in-memory snapshot is the converter's sole input.
    with tarfile.open(fileobj=io.BytesIO(raw), mode="r:") as source:
        members = {m.name.removeprefix("./").rstrip("/"): m for m in source if m.isfile()}
        def blob(identifier):
            name = "blobs/sha256/" + identifier.removeprefix("sha256:")
            require(name in members and members[name].size <= evidence.MAX_BLOB, "missing_frozen_blob")
            with source.extractfile(members[name]) as stream:
                data = stream.read(evidence.MAX_BLOB + 1)
            require(digest(data) == identifier.removeprefix("sha256:"), "changed_frozen_blob")
            return data
        config = blob(identity["config_digest"])
        layers = []
        for layer in identity["layers"]:
            encoded = blob(layer["digest"])
            stream = gzip.GzipFile(fileobj=io.BytesIO(encoded)) if encoded.startswith(b"\x1f\x8b") else io.BytesIO(encoded)
            with stream:
                data = stream.read(evidence.MAX_LAYER + 1)
            require(len(data) <= evidence.MAX_LAYER and "sha256:" + digest(data) == layer["diff_id"], "converted_layer_mismatch")
            layers.append((layer["diff_id"].removeprefix("sha256:") + "/layer.tar", data))
    config_name = identity["config_digest"].removeprefix("sha256:") + ".json"
    manifest = json.dumps([{"Config": config_name, "RepoTags": [tag], "Layers": [name for name, _ in layers]}], sort_keys=True).encode()
    fd = os.open(destination, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "wb") as output, tarfile.open(fileobj=output, mode="w") as target:
        for name, data in [(config_name, config), ("manifest.json", manifest), *layers]:
            member = tarfile.TarInfo(name)
            member.size = len(data)
            member.mode = 0o600
            member.mtime = 0
            target.addfile(member, io.BytesIO(data))
    return receipt, digest(Path(destination).read_bytes())


class Docker:
    def __init__(self, private):
        self.private = private
        self.capacity = None

    def call(self, args, *, check=True, timeout=90):
        if tuple(args[:2]) in MUTATIONS:
            require(self.capacity is not None, "daemon_capacity_not_proved")
            daemon_capacity(self, self.capacity)
        result = subprocess.run(["docker", *args], capture_output=True, timeout=timeout)
        with (self.private / "docker.stdout").open("ab") as output:
            output.write(result.stdout)
        with (self.private / "docker.stderr").open("ab") as output:
            output.write(result.stderr)
        if check:
            require(result.returncode == 0, "docker_operation_failed")
        return result

    def value(self, args):
        try:
            return evidence.json_value(self.call(args).stdout)
        except Exception:
            raise Blocked("invalid_docker_response") from None

    def inventory(self):
        ids = self.call(["container", "ls", "--all", "--quiet", "--no-trunc"]).stdout.decode().split()
        require(all(ID.fullmatch(value) for value in ids), "invalid_inventory")
        result = {}
        for identifier in ids:
            # Never read other containers' environment, config, mounts or logs.
            fields = ["Status", "Running", "StartedAt", "FinishedAt", "Restarting", "Paused"]
            template = "{" + ",".join('"' + key + '":{{json .State.' + key + '}}' for key in fields) + ',"RestartCount":{{json .RestartCount}},"Image":{{json .Image}}}'
            state = self.value(["container", "inspect", "--format", template, identifier])
            result[identifier] = {key: state[key] for key in (*fields, "RestartCount", "Image")}
        return result


class Resources:
    def __init__(self, docker, nonce, image, root):
        self.docker, self.nonce, self.image = docker, nonce, image
        self.root = root
        self.containers = []
        self.network = None
        self.image_created = False
        self.tag = "ndg-smoke-local:" + nonce

    def record(self):
        value = {"nonce": self.nonce, "image": self.image, "tag": self.tag, "image_created": self.image_created, "network": self.network, "containers": self.containers}
        fd = os.open(self.root / "private" / "resource-journal.jsonl", os.O_WRONLY | os.O_CREAT | os.O_APPEND | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, "wb") as output:
            info = os.fstat(output.fileno())
            require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and info.st_uid == os.getuid() and info.st_mode & 0o777 == 0o600, "unsafe_resource_journal")
            output.write(json.dumps(value, sort_keys=True).encode() + b"\n")
            output.flush()
            os.fsync(output.fileno())

    def own(self, kind, identifier):
        require(ID.fullmatch(identifier), "invalid_resource_id")
        data = self.docker.value([kind, "inspect", identifier])
        require(isinstance(data, list) and len(data) == 1 and data[0].get("Id") == identifier, "resource_identity_changed")
        value = data[0]
        labels = value.get("Labels", {}) if kind == "network" else value.get("Config", {}).get("Labels", {})
        require(labels.get(LABEL) == self.nonce, "resource_ownership_unknown")
        if kind == "container":
            require(value.get("Image") == self.image, "container_image_changed")
            config = value.get("HostConfig", {})
            require(config.get("NetworkMode") == self.network and config.get("ReadonlyRootfs") is True and config.get("Privileged") is False and config.get("Memory") == 384 * 1024 * 1024 and config.get("NanoCpus") == 1_000_000_000 and config.get("PidsLimit") == 128 and config.get("CapDrop") == ["ALL"], "container_isolation_changed")
            require(value.get("Config", {}).get("User") == "10001:10001" and config.get("SecurityOpt") in (["no-new-privileges"], ["no-new-privileges:true"]) and config.get("Tmpfs") == {"/tmp": "rw,nosuid,nodev,noexec,size=64m"}, "container_security_options_changed")
            mounts = value.get("Mounts", [])
            actual = {(m.get("Source"), m.get("Destination"), m.get("RW")) for m in mounts if m.get("Type") == "bind"}
            expected = {(str(self.root / "source"), "/sources/source_a", False), (str(self.root / "config"), "/config", False), (str(self.root / "state"), "/data", True)}
            require(actual == expected and all(m.get("Type") == "bind" or m.get("Type") == "tmpfs" and m.get("Destination") == "/tmp" for m in mounts), "container_mounts_changed")
        else:
            require(value.get("Internal") is True and value.get("Driver") == "bridge", "network_isolation_changed")
        return value

    def create_network(self):
        result = self.docker.call(["network", "create", "--driver", "bridge", "--internal", "--label", LABEL + "=" + self.nonce, "ndg-smoke-" + self.nonce])
        identifier = result.stdout.decode().strip()
        require(ID.fullmatch(identifier), "network_id_unknown")
        self.network = identifier
        self.record()
        self.own("network", identifier)

    def create_container(self, root, port=None):
        require(root == self.root, "foreign_fixture_root")
        require(self.network is not None, "missing_private_network")
        self.own("network", self.network)
        args = ["container", "create", "--label", LABEL + "=" + self.nonce, "--network", self.network,
                "--user", "10001:10001", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
                "--pids-limit=128", "--memory=384m", "--cpus=1", "--restart=no",
                "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=64m"]
        for name, target, readonly in [("source", "/sources/source_a", True), ("config", "/config", True), ("state", "/data", False)]:
            args += ["--mount", "type=bind,src=" + str(root / name) + ",dst=" + target + (",readonly" if readonly else "")]
        if port is not None:
            require(type(port) is int and 1024 <= port <= 65535, "invalid_loopback_port")
            args += ["--publish", "127.0.0.1:" + str(port) + ":8080"]
        args.append(self.image)
        identifier = self.docker.call(args).stdout.decode().strip()
        require(ID.fullmatch(identifier), "container_id_unknown")
        self.containers.append(identifier)
        self.record()
        self.own("container", identifier)
        return identifier

    def transition(self, identifier, operation):
        require(identifier in self.containers and operation in {"start", "stop"}, "unowned_transition")
        self.own("container", identifier)
        args = ["container", operation]
        if operation == "stop":
            args += ["--time", "60"]
        return self.docker.call([*args, identifier])

    def cleanup(self):
        # A failed ownership check stops cleanup. Never discover by prefix.
        for identifier in reversed(self.containers):
            self.own("container", identifier)
            self.docker.call(["container", "rm", "--force", identifier])
        if self.network is not None:
            value = self.own("network", self.network)
            require(not value.get("Containers"), "network_has_foreign_endpoint")
            self.docker.call(["network", "rm", self.network])
        if self.image_created:
            value = self.docker.value(["image", "inspect", self.image])[0]
            require(value.get("Id") == self.image and value.get("RepoTags") == [self.tag] and not value.get("RepoDigests"), "image_ownership_unknown")
            users = self.docker.call(["container", "ls", "--all", "--quiet", "--no-trunc", "--filter", "ancestor=" + self.image]).stdout.strip()
            require(not users, "image_has_foreign_container")
            # Remove the nonce reference only, never rm an existing image ID.
            self.docker.call(["image", "rm", self.tag])


def check_existing(before, after):
    require(all(after.get(identifier) == state for identifier, state in before.items()), "existing_container_state_changed")


def fixture(root, port):
    if not (root / "private").exists():
        (root / "private").mkdir(mode=0o700)
    for name in ["source", "state", "config"]:
        (root / name).mkdir(mode=0o700)
    marker = "synthetic_" + secrets.token_hex(16)
    for pair in range(3):
        for copy in range(2):
            private_write(root / "source" / (marker + "_" + str(pair) + "_" + str(copy)), ("pair_" + str(pair)).encode() * 50)
    private_write(root / "source" / (marker + "_singleton"), b"synthetic singleton")
    password = secrets.token_hex(32)
    private_write(root / "config" / "credential", password.encode())
    settings = {"listen": "0.0.0.0:8080", "origin": "http://127.0.0.1:" + str(port), "state": "/data", "credential_file": "/config/credential", "sources": {"source_a": "/sources/source_a"}}
    private_write(root / "config" / "server.json", json.dumps(settings).encode())
    return settings, password, marker


def source_hashes(root):
    values = {}
    for member in (root / "source").iterdir():
        info = member.lstat()
        require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1, "fixture_source_changed")
        values[member.name] = digest(member.read_bytes())
    require(len(values) == 7, "fixture_coverage_changed")
    return values


def exercise(resources, root, port, settings, password, marker, source):
    before = source_hashes(root)
    private_write(root / "private" / "source-before.json", json.dumps(before, sort_keys=True).encode())
    jar = http.cookiejar.CookieJar()
    client = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(jar))
    csrf = ""
    def capture(raw):
        with (root / "private" / "http.responses").open("ab") as output:
            output.write(raw + b"\n")
        require(len(raw) <= 1024 * 1024 and all(value.encode() not in raw for value in [marker, password, str(root), "/sources/source_a"]), "http_privacy_window_failed")
    def request(path, body=None):
        headers = {} if body is None else {"Origin": settings["origin"], "Content-Type": "application/json", "X-CSRF-Token": csrf}
        data = None if body is None else json.dumps(body).encode()
        try:
            with client.open(urllib.request.Request(settings["origin"] + path, data=data, headers=headers), timeout=5) as response:
                raw = response.read(1024 * 1024 + 1)
        except urllib.error.HTTPError as error:
            capture(error.read(1024 * 1024 + 1))
            raise
        capture(raw)
        return evidence.json_value(raw)
    def ready():
        for _ in range(60):
            try:
                request("/healthz")
                return
            except (OSError, urllib.error.URLError):
                time.sleep(0.5)
        raise Blocked("own_container_health_timeout")
    identifier = resources.create_container(root, port)
    resources.transition(identifier, "start")
    own = resources.own("container", identifier)
    require(own["NetworkSettings"]["Ports"]["8080/tcp"] == [{"HostIp": "127.0.0.1", "HostPort": str(port)}], "loopback_binding_changed")
    networks = own["NetworkSettings"].get("Networks", {})
    require(len(networks) == 1 and next(iter(networks.values())).get("NetworkID") == resources.network, "active_network_binding_changed")
    ready()
    try:
        request("/api/scan", {"alias": "source_a"})
        raise Failed("unauthenticated_scan_accepted")
    except urllib.error.HTTPError as error:
        require(error.code == 401, "unexpected_auth_result")
    csrf = request("/api/login", {"password": password})["csrf"]
    version = request("/api/version")
    require(version.get("commit") == source["commit"], "runtime_source_mismatch")
    require(version.get("version") == source["core_version"] and version.get("channel") == "web-experimental" and version.get("build_time") == source["build_time"], "runtime_version_mismatch")
    for endpoint in ["/api/" + area + "/" + operation for area in ["quarantine", "restore", "purge"] for operation in ["create", "approve", "dry-run", "execute"]]:
        try:
            request(endpoint, {})
            raise Failed("dangerous_api_accepted")
        except urllib.error.HTTPError as error:
            require(error.code == 403, "unexpected_execution_refusal")
    request("/api/scan", {"alias": "source_a", "resume": False})
    for _ in range(80):
        result = request("/api/status", {"alias": "source_a"})
        if result.get("state") == "COMPLETED":
            break
        require(result.get("state") not in {"FAILED", "PAUSED_NETWORK"}, "scan_not_completed")
        time.sleep(0.25)
    require(result.get("state") == "COMPLETED" and result.get("discovered") == result.get("processed") == result.get("scanned") == 7 and result.get("failed") == 0 and result.get("active") == 7 and result.get("quick") == 7 and result.get("full") == 6 and len(result.get("groups", [])) == 3 and all(g.get("copies") == 2 and g.get("bytes") == 300 for g in result["groups"]) and result.get("checkpoint") == "completed" and result.get("missing") == result.get("unavailable") == 0, "coverage_incomplete")
    second = resources.create_container(root)
    resources.transition(second, "start")
    state = resources.docker.call(["container", "wait", second], timeout=15).stdout.strip()
    require(state.isdigit() and int(state) != 0, "second_writer_not_refused")
    require(resources.own("container", second).get("State", {}).get("OOMKilled") is False, "second_writer_cause_unconfirmed")
    resources.transition(identifier, "stop")
    # Disambiguate the second process's failure: the same container/config must
    # remain running when the first owner's lock has been legally released.
    resources.transition(second, "start")
    time.sleep(1)
    require(resources.own("container", second).get("State", {}).get("Running") is True, "second_writer_cause_unconfirmed")
    resources.transition(second, "stop")
    resources.transition(identifier, "start")
    ready()
    jar.clear()
    csrf = request("/api/login", {"password": password})["csrf"]
    result = request("/api/status", {"alias": "source_a"})
    require(result.get("scanned") == 7 and result.get("active") == 7 and result.get("quick") == 7 and result.get("full") == 6 and len(result.get("groups", [])) == 3 and result.get("checkpoint") == "completed" and result.get("missing") == result.get("unavailable") == result.get("failed") == 0, "restart_state_lost")
    after = source_hashes(root)
    private_write(root / "private" / "source-after.json", json.dumps(after, sort_keys=True).encode())
    require(after == before, "source_bytes_changed")
    for owned in resources.containers:
        resources.own("container", owned)
        log = resources.docker.call(["container", "logs", owned])
        raw = log.stdout + log.stderr + json.dumps(result).encode()
        require(all(value.encode() not in raw for value in [marker, password, str(root), "/sources/source_a"]), "privacy_window_failed")
    return {"source_files": 7, "quick": 7, "full": 6, "groups": 3, "checkpoint": "completed", "missing": 0, "unavailable": 0, "source_sha_unchanged": True, "single_writer_refused": True, "own_restart": True, "privacy_scope": "own_container_stdout_stderr_and_all_received_http_bodies_only"}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute-disposable", action="store_true")
    parser.add_argument("--bundle", type=Path)
    parser.add_argument("--operator-receipt-fd", type=int)
    args = parser.parse_args(argv)
    if not args.execute_disposable:
        print("nas-smoke: protocol_only_no_docker_no_files_no_credentials")
        return 0
    resources = None
    docker = None
    before = None
    root = None
    receipt_written = False
    handoff_fd = None
    nonce = None
    receipt_sha = None
    handoff_identity = None
    result = {"status": "BLOCKED", "scope": "fnos_docker_disposable_readonly_host_only", "fpk": "NOT_RUN", "smb": "NOT_RUN", "real_nas_sources": "NOT_RUN", "release": "NOT_AUTHORIZED"}
    try:
        handoff_fd = operator_pipe(args.operator_receipt_fd)
        result["harness_sha256"] = digest(Path(__file__).read_bytes())
        require(platform.system() == "Linux" and platform.machine() == "x86_64" and os.geteuid() == 0, "explicit_linux_root_required")
        require(args.bundle is not None, "frozen_evidence_required")
        require(not os.environ.get("DOCKER_HOST") and os.environ.get("DOCKER_CONTEXT", "default") == "default", "remote_docker_forbidden")
        expected = FROZEN
        memory = Path("/proc/meminfo").read_text()
        match = re.search(r"^MemAvailable:\s+(\d+) kB$", memory, re.MULTILINE)
        require(match is not None and int(match[1]) >= 1024 * 1024, "insufficient_available_memory")
        os.umask(0o077)
        parent = temporary_parent()
        root = Path(tempfile.mkdtemp(prefix="ndg-host-smoke-", dir=str(parent))).resolve(strict=True)
        (root / "private").mkdir(mode=0o700)
        require(shutil.disk_usage(root).free >= 256 * 1024 * 1024, "insufficient_temporary_disk")
        # Kernel assigns an unused random loopback port; a later race fails bind
        # closed at container start, never touches the competing listener.
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", 0))
            port = probe.getsockname()[1]
        settings, password, marker = fixture(root, port)
        docker = Docker(root / "private")
        context = docker.call(["context", "inspect", "default", "--format", "{{.Endpoints.docker.Host}}"])
        require(context.stdout.strip() == b"unix:///var/run/docker.sock", "local_docker_required")
        docker.capacity = daemon_capacity(docker)
        before = docker.inventory()
        nonce = secrets.token_hex(16)
        resources = Resources(docker, nonce, expected["config_digest"], root)
        resources.record()
        # This private initial handoff precedes all daemon mutations. It lets
        # the operator retain the exact owned fixture after timeout, without glob.
        handoff_identity = operator_handoff(handoff_fd, "initial", root, nonce)
        receipt, converted_sha = docker_archive(args.bundle, expected, root / "private" / "docker-load.tar", resources.tag)
        present = docker.call(["image", "inspect", expected["config_digest"]], check=False)
        if present.returncode != 0:
            images = docker.call(["image", "ls", "--quiet", "--no-trunc"]).stdout.decode().split()
            require(all(re.fullmatch(r"sha256:[0-9a-f]{64}", value) for value in images) and expected["config_digest"] not in images, "image_inventory_unconfirmed")
            require(docker.call(["image", "inspect", resources.tag], check=False).returncode != 0, "private_tag_exists")
            docker.call(["image", "load", "--input", str(root / "private" / "docker-load.tar")])
            resources.image_created = True
            resources.record()
        else:
            require(evidence.json_value(present.stdout)[0].get("Id") == expected["config_digest"], "existing_image_identity_mismatch")
        loaded = docker.value(["image", "inspect", expected["config_digest"]])[0]
        require(loaded.get("Id") == expected["config_digest"] and loaded.get("Architecture") == "amd64", "loaded_image_mismatch")
        labels = loaded.get("Config", {}).get("Labels", {})
        require(labels.get("org.opencontainers.image.revision") == expected["commit"] and labels.get("com.ndg.source-tree") == expected["tree"] and loaded.get("RootFS", {}).get("Layers") == [layer["diff_id"] for layer in receipt["image"]["layers"]], "loaded_content_identity_mismatch")
        for name in ["source", "state", "config"]:
            for member in [root / name, *(root / name).iterdir()]:
                os.chown(member, 10001, 10001, follow_symlinks=False)
        resources.create_network()
        result["checks"] = exercise(resources, root, port, settings, password, marker, receipt["source"])
        result.update(status="PASS", source=receipt["source"], image=receipt["image"], converted_archive_sha256=converted_sha)
    except Exception as error:
        if isinstance(error, Failed):
            result["status"] = "FAIL"
        result["reason"] = str(error) if isinstance(error, Blocked) else "validation_or_tool_failed"
    finally:
        result["test_status"] = result["status"]
        if "reason" in result:
            result["test_reason"] = result["reason"]
        if root is not None:
            try:
                private_write(root / "private" / "pre-cleanup-receipt.json", json.dumps(result, sort_keys=True).encode())
            except Exception:
                result.update(status="BLOCKED", reason="private_precleanup_receipt_not_persisted")
        try:
            if resources is not None:
                for identifier in resources.containers:
                    resources.own("container", identifier)
                    docker.call(["container", "logs", identifier], check=False)
                resources.cleanup()
        except Exception:
            result.update(status="BLOCKED", reason="ownership_cleanup_or_existing_state_unconfirmed")
        if before is not None:
            try:
                check_existing(before, docker.inventory())
            except Blocked as error:
                if str(error) == "existing_container_state_changed" and result["status"] != "BLOCKED":
                    result.update(status="INCONCLUSIVE", reason="existing_state_changed_without_intervention")
                else:
                    result.update(status="BLOCKED", reason="existing_state_unconfirmed")
            except Exception:
                result.update(status="BLOCKED", reason="existing_state_unconfirmed")
        if root is not None:
            try:
                encoded = json.dumps(result, sort_keys=True).encode()
                private_write(root / "private" / "result.json", encoded)
                receipt_written = True
                receipt_sha = digest(encoded)
            except Exception:
                result.update(status="BLOCKED", reason="private_receipt_not_persisted")
        try:
            operator_handoff(handoff_fd, "final", root, nonce, result=result, receipt_sha=receipt_sha, identity=handoff_identity)
        except Exception:
            # A persisted test receipt is not proof the operator received it.
            # Preserve it; fail the operation closed without rewriting evidence.
            result.update(status="BLOCKED", reason="operator_handoff_failed")
    boundary = "private_evidence_retained" if receipt_written else "preflight_no_fixture" if root is None else "private_partial_evidence_only"
    reason = result.get("reason", "")
    safe_reason = reason if re.fullmatch(r"[a-z0-9_]{1,80}", reason) else "validation_or_tool_failed"
    print("nas-smoke: " + result["status"].lower() + "_disposable_host_only_" + boundary + (":" + safe_reason if result["status"] != "PASS" else ""))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    sys.exit(main())
