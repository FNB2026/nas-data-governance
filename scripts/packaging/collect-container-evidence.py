#!/usr/bin/env python3
"""GitHub-hosted native runner experiment only. No registry push or attestation."""
import datetime
import hashlib
import importlib
import io
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import sys
import tarfile
import urllib.request

sys.dont_write_bytecode = True
evidence = importlib.import_module("container-evidence")
ROOT = Path(__file__).resolve().parents[2]
stage = "preflight"


def require(value, code):
    if not value:
        raise evidence.Rejected(code)


def run(args, private, *, cwd=ROOT, timeout=1800, env=None):
    result = subprocess.run(args, cwd=cwd, env=env, capture_output=True, timeout=timeout)
    # Raw diagnostics stay private, never included in the upload allowlist.
    with (private / (stage + ".stdout")).open("ab") as output:
        output.write(result.stdout)
    with (private / (stage + ".stderr")).open("ab") as output:
        output.write(result.stderr)
    return result


def public_json(path, value, private_root):
    def scrub(item):
        if isinstance(item, str):
            result = item.replace(str(ROOT), "<SOURCE_ROOT>").replace(str(private_root.parent), "<RUNNER_TEMP>")
            if result.startswith("/"):
                result = "<LOCAL_PATH_REDACTED>"
            for name, secret in os.environ.items():
                if re.search(r"TOKEN|SECRET|PASSWORD|PRIVATE_KEY", name) and len(secret) >= 8:
                    require(secret not in result, "secret_in_tool_output")
            require("ndg-disposable-" not in result, "fixture_path_in_tool_output")
            return result
        if isinstance(item, list):
            return [scrub(value) for value in item]
        if isinstance(item, dict):
            values = {}
            for key, value in item.items():
                safe_key = scrub(key)
                require(safe_key not in values, "redaction_key_collision")
                values[safe_key] = scrub(value)
            return values
        return item
    path.write_text(json.dumps(scrub(value), indent=2, sort_keys=True) + "\n")


def fetch_tool(name, lock, architecture, target):
    description = lock["tools"][name]
    archive = description["archives"][architecture]
    checksums = urllib.request.urlopen(description["checksum_url"], timeout=60).read(1024 * 1024).decode()
    matches = [line.split()[0] for line in checksums.splitlines() if len(line.split()) == 2 and line.split()[1].lstrip("*") == archive["filename"]]
    require(matches == [archive["sha256"]], "publisher_checksum_changed")
    with urllib.request.urlopen(archive["url"], timeout=120) as response:
        data = response.read(200 * 1024 * 1024 + 1)
    require(len(data) <= 200 * 1024 * 1024 and hashlib.sha256(data).hexdigest() == archive["sha256"], "tool_checksum_mismatch")
    if name != "buildx":
        with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as tar:
            entries = [entry for entry in tar if entry.name == name]
            require(len(entries) == 1 and entries[0].isfile() and entries[0].size <= 200 * 1024 * 1024, "unsafe_tool_archive")
            with tar.extractfile(entries[0]) as source:
                data = source.read()
    target.write_bytes(data)
    target.chmod(0o700)
    return {"version": description["version"], "download_sha256": archive["sha256"], "binary_sha256": hashlib.sha256(data).hexdigest(), "publisher_signature": "NOT_VERIFIED"}


def main():
    global stage
    architecture = os.environ.get("NDG_EVIDENCE_ARCH", "")
    require(os.environ.get("GITHUB_ACTIONS") == "true" and os.environ.get("GITHUB_REPOSITORY") == "FNB2026/nas-data-governance", "github_runner_only")
    require(architecture in {"amd64", "arm64"} and platform.system() == "Linux" and platform.machine() == {"amd64": "x86_64", "arm64": "aarch64"}[architecture], "native_linux_runner_required")
    os.umask(0o077)
    temporary = Path(os.environ["RUNNER_TEMP"]).resolve(strict=True)
    work = temporary / "ndg-container-evidence"
    work.mkdir(mode=0o700)  # Never reuse or clean an existing namespace.
    private = work / "private"
    public = work / "public"
    private.mkdir(mode=0o700)
    public.mkdir(mode=0o700)
    try:
        require(run(["git", "status", "--porcelain=v1"], private).stdout.strip() == b"", "dirty_checkout")
        commit = run(["git", "rev-parse", "HEAD"], private).stdout.decode().strip()
        tree = run(["git", "rev-parse", "HEAD^{tree}"], private).stdout.decode().strip()
        require(commit == os.environ["GITHUB_SHA"], "checkout_identity_mismatch")
        epoch = int(run(["git", "show", "-s", "--format=%ct", "HEAD"], private).stdout.strip())
        build_time = datetime.datetime.fromtimestamp(epoch, datetime.timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")
        source = {"commit": commit, "tree": tree, "core_version": (ROOT / "packaging/docker/PREVIEW_VERSION").read_text().strip(), "build_time": build_time, "channel": "web-experimental"}
        require(re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[a-z0-9.-]+)?", source["core_version"]), "invalid_core_version")
        lock = json.loads((ROOT / "packaging/docker/evidence-tools.lock.json").read_text())
        docker_config = private / "docker-config"
        plugins = docker_config / "cli-plugins"
        plugins.mkdir(parents=True, mode=0o700)
        os.environ["DOCKER_CONFIG"] = str(docker_config)
        require(not os.environ.get("DOCKER_HOST") and os.environ.get("DOCKER_CONTEXT", "default") == "default", "remote_docker_forbidden")
        stage = "tools"
        tools = {}
        for name in ["buildx", "syft", "grype"]:
            target = plugins / "docker-buildx" if name == "buildx" else private / name
            tools[name] = fetch_tool(name, lock, architecture, target)
            args = ["docker", "buildx", "version"] if name == "buildx" else [str(target), "--version"]
            version = run(args, private)
            require(version.returncode == 0 and lock["tools"][name]["version"].removeprefix("v").encode() in version.stdout, "tool_version_mismatch")
            tools[name]["reported_version"] = version.stdout.decode().strip()
        context = run(["docker", "context", "inspect", "default", "--format", "{{.Endpoints.docker.Host}}"], private)
        require(context.returncode == 0 and context.stdout.strip() == b"unix:///var/run/docker.sock", "local_runner_docker_required")
        tools["docker_engine"] = run(["docker", "version", "--format", "{{json .Server.Version}}"], private).stdout.decode().strip()
        tools["buildkit_image"] = lock["buildkit_image"]
        tools["govulncheck_module"] = lock["govulncheck_module"]
        stage = "build"
        builder = "ndg-evidence-" + architecture
        created = run(["docker", "buildx", "create", "--name", builder, "--driver", "docker-container", "--driver-opt", "image=" + lock["buildkit_image"], "--use"], private)
        require(created.returncode == 0, "builder_creation_failed")
        args = ["docker", "buildx", "build", "--builder", builder, "--platform", "linux/" + architecture, "--tag", "ndg-disposable:test", "--provenance=false", "--sbom=false", "--output", "type=docker", "--output", "type=oci,dest=" + str(public / "image.oci.tar"), "--build-arg", "TARGETARCH=" + architecture, "--build-arg", "SOURCE_SHA=" + commit, "--build-arg", "SOURCE_TREE=" + tree, "--build-arg", "CORE_VERSION=" + source["core_version"], "--build-arg", "SOURCE_BUILD_TIME=" + build_time, "--build-arg", "SOURCE_DATE_EPOCH=" + str(epoch), "-f", "packaging/docker/Dockerfile", "."]
        require(run(args, private).returncode == 0, "image_build_failed")
        identity = evidence.image_identity(public / "image.oci.tar", source, architecture)
        loaded = run(["docker", "image", "inspect", "--format", "{{.Id}}", "ndg-disposable:test"], private)
        require(loaded.returncode == 0 and loaded.stdout.decode().strip() == identity["config_digest"], "loaded_image_identity_mismatch")
        stage = "runtime"
        accepted = run(["sudo", "env", "NDG_EXPECT_ARCH=" + architecture, "NDG_EXPECT_VERSION=" + source["core_version"], "NDG_EXPECT_BUILD_TIME=" + build_time, "python3", "scripts/packaging/test-linux-container.py"], private)
        require(accepted.returncode == 0 and accepted.stdout.startswith(b"PASS: disposable native "), "synthetic_runtime_failed")
        public_json(public / "runtime.json", {"status": "PASS", "loaded_config_digest": identity["config_digest"], "native_machine": platform.machine()}, private)
        stage = "sbom"
        for origin, selector in [("image", "oci-archive:" + str(public / "image.oci.tar")), ("source", "dir:" + str(ROOT))]:
            for standard, output in [("cyclonedx", "cyclonedx-json"), ("spdx", "spdx-json")]:
                args = [str(private / "syft"), selector, "-o", output, "--exclude", "**/.git/**", "--exclude", "**/node_modules/**", "--exclude", "**/bin/**", "--exclude", "**/dist/**"]
                # Syft source defaults can omit dev dependencies. Make the
                # reviewed scope explicit; npm audit cannot replace inventory.
                syft_env = dict(os.environ, SYFT_JAVASCRIPT_INCLUDE_DEV_DEPENDENCIES="true", SYFT_CHECK_FOR_APP_UPDATE="false")
                result = run(args, private, env=syft_env)
                require(result.returncode == 0, "sbom_failed")
                public_json(public / (origin + "." + standard + ".json"), evidence.json_value(result.stdout), private)
        frontend_lock = ROOT / "cmd/ndg-desktop/frontend/package-lock.json"
        inventory = evidence.verify_frontend_sbom_inventory(public, frontend_lock)
        scans = {}
        stage = "npm"
        npm = run(["docker", "run", "--rm", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--tmpfs", "/tmp:rw,nosuid,nodev,size=128m", "--user", "10001:10001", "--env", "HOME=/tmp", "--volume", str(ROOT / "cmd/ndg-desktop/frontend") + ":/source:ro", "--workdir", "/source", lock["node_audit_image"], "npm", "audit", "--package-lock-only", "--json", "--audit-level=high"], private)
        npm_data = evidence.json_value(npm.stdout)
        counts = npm_data.get("metadata", {}).get("vulnerabilities", {})
        npm_clear = npm.returncode == 0 and counts.get("high") == 0 and counts.get("critical") == 0
        scans["npm"] = {"exit_code": npm.returncode, "status": "PASS" if npm_clear else "BLOCKED", "scope": "frontend_lock_includes_build_dependencies"}
        public_json(public / "npm-audit.json", npm_data, private)
        stage = "govulncheck"
        go_environment = dict(os.environ, GOSUMDB="sum.golang.org")
        go = run(["go", "run", lock["govulncheck_module"], "-json", "./cmd/ndg-server"], private, env=go_environment)
        # govulncheck emits pretty-printed JSON objects, not line JSON.
        decoder = json.JSONDecoder()
        text = go.stdout.decode()
        go_data = []
        while text.strip():
            item, end = decoder.raw_decode(text.lstrip())
            go_data.append(item)
            text = text.lstrip()[end:]
        source_findings = any("finding" in item for item in go_data)
        scans["govulncheck"] = {"exit_code": go.returncode, "status": "PASS" if go.returncode == 0 and bool(go_data) and not source_findings else "BLOCKED", "scope": "reachable_cmd_ndg_server_source_linux"}
        public_json(public / "govulncheck.json", go_data, private)
        stage = "grype"
        grype = run([str(private / "grype"), "oci-archive:" + str(public / "image.oci.tar"), "--fail-on", "high", "-o", "json"], private)
        grype_data = evidence.json_value(grype.stdout)
        public_json(public / "grype.json", grype_data, private)
        require(isinstance(grype_data.get("matches"), list) and bool(grype_data.get("descriptor", {}).get("db")), "missing_vulnerability_database_evidence")
        scans["grype"] = {"exit_code": grype.returncode, "status": "PASS" if grype.returncode == 0 else "BLOCKED", "scope": "final_oci_runtime_image"}
        public_json(public / "tools.json", tools, private)
        require(run(["git", "status", "--porcelain=v1"], private).stdout.strip() == b"", "checkout_changed_during_evidence")
        receipt = {"schema": 1, "scope": "experimental_ci_evidence_only", "source": source, "architecture": architecture, "image": identity, "signed_attestation": "NOT_SIGNED", "reproducibility": "NOT_RUN", "source_sbom_inventory": inventory, "scans": scans, "workflow_run": {"id": os.environ["GITHUB_RUN_ID"], "attempt": os.environ["GITHUB_RUN_ATTEMPT"], "source_sha": os.environ["GITHUB_SHA"]}, "files": {name: hashlib.sha256((public / name).read_bytes()).hexdigest() for name in evidence.ARTIFACTS}}
        public_json(public / "receipt.json", receipt, private)
        evidence.verify_bundle(public, expected_commit=commit, expected_tree=tree, expected_architecture=architecture, expected_frontend_lock=frontend_lock)
        require(all(scan["status"] == "PASS" for scan in scans.values()), "vulnerability_gate_blocked")
        print("container-evidence: native_runtime_and_unsigned_content_gate_pass_not_release")
        return 0
    except Exception as error:
        # No incomplete image is accepted. Retain only already-sanitized JSON
        # reports plus an explicit failure classification for independent triage.
        if not (public / "receipt.json").is_file():
            for member in public.iterdir():
                require(member.is_file() and not member.is_symlink(), "unsafe_partial_evidence")
                if member.name not in evidence.ARTIFACTS or member.suffix != ".json":
                    member.unlink()  # Only this invocation's fresh CI output.
            public_json(public / "failure.json", {"schema": 1, "stage": stage, "status": "BLOCKED", "scope": "incomplete_not_accepted", "image_retained": False, "retained_files": sorted(member.name for member in public.iterdir()), "reason": str(error) if isinstance(error, evidence.Rejected) else "tool_or_validation_failed"}, private)
        print("container-evidence: gate_blocked_private_diagnostics_not_uploaded", file=sys.stderr)
        return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception:
        print("container-evidence: runner_preflight_rejected", file=sys.stderr)
        sys.exit(1)
