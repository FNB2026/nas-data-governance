#!/usr/bin/env python3
"""Only CI/local disposable containers with fresh owned state; never existing services."""
import hashlib, http.cookiejar, json, os, pathlib, platform, re, secrets, subprocess, tempfile, time, urllib.request, urllib.error

def run(*args, check=True):
    return subprocess.run(args, check=check, capture_output=True, text=True, timeout=90)

architecture = os.environ.get("NDG_EXPECT_ARCH", "amd64")
assert architecture in {"amd64", "arm64"}
assert platform.machine() == {"amd64": "x86_64", "arm64": "aarch64"}[architecture]
assert run("docker", "image", "inspect", "--format", "{{.Architecture}}", "ndg-disposable:test").stdout.strip() == architecture

with tempfile.TemporaryDirectory(prefix="ndg-disposable-") as temp:
    root = pathlib.Path(temp); source = root / "source"; state = root / "state"; config = root / "config"
    for p in [source, state, config]:
        p.mkdir(mode=0o700)
    for i in range(3):
        for j in range(2):
            (source / f"synthetic_marker_{i}_{j}").write_bytes((f"pair_{i}" * 50).encode())
    (source / "synthetic_singleton").write_bytes(b"singleton")
    before = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in source.iterdir()}
    password = secrets.token_hex(32)
    (config / "credential").write_text(password); os.chmod(config / "credential", 0o600)
    settings = {"listen": "0.0.0.0:8080", "origin": "http://127.0.0.1:18083", "state": "/data", "credential_file": "/config/credential", "sources": {"source_a": "/sources/source_a"}}
    (config / "server.json").write_text(json.dumps(settings)); os.chmod(config / "server.json", 0o600)
    # CI fixture ownership only. No host NAS or production volume is touched.
    for p in [root, source, state, config, *config.iterdir()]:
        os.chown(p, 10001, 10001)
    name = "ndg-disposable-" + secrets.token_hex(6)
    common = ["--user", "10001:10001", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=128", "--memory=1g", "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=64m", "-v", f"{source}:/sources/source_a:ro", "-v", f"{state}:/data:rw", "-v", f"{config}:/config:ro"]
    jar = http.cookiejar.CookieJar(); client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar)); csrf = ""
    def request(path, body=None):
        headers = {}
        if body is not None: headers = {"Origin": settings["origin"], "Content-Type": "application/json", "X-CSRF-Token": csrf}
        req = urllib.request.Request(settings["origin"] + path, data=None if body is None else json.dumps(body).encode(), headers=headers)
        with client.open(req, timeout=5) as response: return json.load(response)
    def ready():
        for _ in range(60):
            try: request("/healthz"); return
            except (OSError, urllib.error.URLError): time.sleep(0.5)
        raise AssertionError("container did not become healthy")
    try:
        run("docker", "run", "-d", "--name", name, "-p", "127.0.0.1:18083:8080", *common, "ndg-disposable:test"); ready()
        expected_sha = run("git", "rev-parse", "HEAD").stdout.strip()
        image_sha = run("docker", "inspect", "--format", '{{index .Config.Labels "org.opencontainers.image.revision"}}', name).stdout.strip()
        assert image_sha == expected_sha
        with client.open(settings["origin"] + "/", timeout=5) as response:
            html = response.read().decode()
            assert response.headers["Content-Security-Policy"] and 'id="root"' in html and "assets not built" not in html
        assets = re.findall(r'(?:src|href)="(/assets/[^\"]+)"', html)
        assert any(asset.endswith(".js") for asset in assets)
        for asset in assets:
            with client.open(settings["origin"] + asset, timeout=5) as response:
                assert response.status == 200 and len(response.read()) > 0
        try: request("/api/scan", {"alias": "source_a"}); raise AssertionError("unauthenticated scan accepted")
        except urllib.error.HTTPError as e: assert e.code == 401
        csrf = request("/api/login", {"password": password})["csrf"]
        identity = request("/api/version")
        assert identity["commit"] == expected_sha and identity["channel"] == "web-experimental"
        if "NDG_EXPECT_VERSION" in os.environ:
            assert identity["version"] == os.environ["NDG_EXPECT_VERSION"]
            assert identity["build_time"] == os.environ["NDG_EXPECT_BUILD_TIME"]
        for endpoint in ["/api/quarantine/execute", "/api/restore/execute", "/api/purge/execute"]:
            try: request(endpoint, {}); raise AssertionError("execution accepted")
            except urllib.error.HTTPError as e: assert e.code == 403
        request("/api/scan", {"alias": "source_a", "resume": False})
        for _ in range(60):
            result = request("/api/status", {"alias": "source_a"})
            if result["state"] == "COMPLETED": break
            assert result["state"] != "FAILED"; time.sleep(0.25)
        assert result["state"] == "COMPLETED" and result["active"] == 7 and result["quick"] == 7 and result["full"] == 6 and len(result["groups"]) == 3
        assert result["missing"] == result["unavailable"] == 0 and result["checkpoint"] == "completed"
        second = name + "-second"
        blocked = run("docker", "run", "--name", second, *common, "ndg-disposable:test", check=False)
        run("docker", "rm", second)
        assert blocked.returncode != 0
        run("docker", "stop", "--time", "60", name); run("docker", "start", name); ready()
        jar.clear(); csrf = request("/api/login", {"password": password})["csrf"]
        result = request("/api/status", {"alias": "source_a"}); assert result["active"] == 7 and result["full"] == 6 and result["checkpoint"] == "completed"
        after = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in source.iterdir()}; assert before == after
        logs = run("docker", "logs", name).stdout + run("docker", "logs", name).stderr
        assert "synthetic_marker" not in logs and password not in logs and str(root) not in logs
        assert "synthetic_marker" not in json.dumps(result) and "/sources/" not in json.dumps(result)
        print(f"PASS: disposable native {architecture} container exact SHA, embedded Web assets, auth, source bytes, 7 quick / 6 full / 3 groups, execution refusal, single writer and restart")
    finally:
        run("docker", "rm", "-f", name, check=False)
