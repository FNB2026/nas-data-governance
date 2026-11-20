#!/usr/bin/env bash
set -euo pipefail
[[ "${GITHUB_ACTIONS:-}" == true && "${RUNNER_ENVIRONMENT:-}" == github-hosted ]]
umask 077
fixture="$(mktemp -d /tmp/ndg-quarantine-ci-XXXXXXXX)"
project="ndg-quarantine-ci-${RANDOM}${RANDOM}"
compose=(docker compose --project-name "$project" -f packaging/docker/compose.quarantine-candidate.yaml)
cleanup() {
  local result=$?
  if (( result != 0 )); then
    "${compose[@]}" ps --all --format json | python3 -c 'import json,sys; print([(x.get("Service"),x.get("State"),x.get("ExitCode")) for line in sys.stdin if line.strip() for x in [json.loads(line)]])' || true
  fi
  "${compose[@]}" down >/dev/null 2>&1 || true
}
trap cleanup EXIT
mkdir -m 0700 "$fixture"/{state,quarantine,source,config}
mkdir -m 0755 "$fixture/run"
mkdir -m 0700 "$fixture"/{quarantine,source}/.ndg-purge-stage
go run ./scripts/qa/quarantine-candidate-fixture "$fixture" > "$fixture/item"
{
  printf '%s' 'ndg-gov-'
  openssl rand -hex 32
} > "$fixture/config/token"
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=10.254.140.2 \
  -addext subjectAltName=IP:10.254.140.2 -keyout "$fixture/config/tls.key" -out "$fixture/config/tls.crt" >/dev/null 2>&1
cp "$fixture/config/token" "$fixture/client-token"
cp "$fixture/config/tls.crt" "$fixture/client.crt"
printf 'protected synthetic sentinel\n' > "$fixture/sentinel"
sha256sum "$fixture/sentinel" > "$fixture/sentinel.sha"
sudo -n chown -R 10002:10002 "$fixture"/{state,quarantine,source,run}
sudo -n chown -R 10001:10001 "$fixture/config"
export NDG_CANDIDATE_STATE="$fixture/state" NDG_CANDIDATE_RUN="$fixture/run"
export NDG_CANDIDATE_SOURCE="$fixture/source"
export NDG_CANDIDATE_WEB_CONFIG="$fixture/config" NDG_CANDIDATE_QUARANTINE="$fixture/quarantine"
: "${NDG_QUARANTINE_IMAGE:?build candidate image first}"
"${compose[@]}" up -d --no-build --pull never >/dev/null
python3 -B - "$fixture" "${compose[@]}" <<'PY'
import hashlib
import json
from pathlib import Path
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.request

base = Path(sys.argv[1])
tls = ssl.create_default_context(cafile=str(base / 'client.crt'))
url = 'https://10.254.140.2:8443'
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=tls))
for _ in range(30):
    try:
        opener.open(url + '/gov/capabilities', timeout=2).close()
    except urllib.error.HTTPError as e:
        if e.code == 401:
            break
    except urllib.error.URLError:
        time.sleep(1)
else:
    raise RuntimeError('candidate TLS endpoint did not become ready')

token = (base / 'client-token').read_text().strip()
item_id, source_one, keeper_one, source_two, keeper_two = (base / 'item').read_text().splitlines()
def request(payload, expected=200, route='/gov/quarantine-purge'):
    req = urllib.request.Request(url + route,
                                 json.dumps(payload).encode(),
                                 {'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token})
    try:
        with opener.open(req, timeout=15) as result:
            assert result.status == expected
            return json.load(result)
    except urllib.error.HTTPError as err:
        assert err.code == expected, (payload['op'], err.code)

plan = request({'op': 'QUARANTINE_PURGE_PLAN', 'item_id': item_id})
def act(suffix, confirmation=None, expected=200):
    data = {'op': 'QUARANTINE_PURGE_' + suffix, 'plan_id': plan['id'], 'digest': plan['digest']}
    if confirmation:
        data['confirmation'] = confirmation
    return request(data, expected)
assert act('CANCEL')['state'] == 'CANCELLED'
act('APPROVE', expected=409)
plan = request({'op': 'QUARANTINE_PURGE_PLAN', 'item_id': item_id})
act('EXECUTE', '永久清理 ' + plan['id'], 409)
assert act('APPROVE')['state'] == 'APPROVED'
assert act('DRY_RUN')['state'] == 'DRY_RUN'
assert act('EXECUTE', '永久清理 ' + plan['id'])['state'] == 'PURGED'
assert act('RECEIPT')['state'] == 'PURGED'
subprocess.run(sys.argv[2:] + ['restart', 'worker'], check=True, capture_output=True)
for _ in range(30):
    try:
        assert act('RECEIPT')['state'] == 'PURGED'
        break
    except AssertionError as err:
        # Only allow the Web gateway's temporary unavailable response.
        if not err.args or err.args[0] != ('QUARANTINE_PURGE_RECEIPT', 503):
            raise
        time.sleep(1)
else:
    raise RuntimeError('persisted receipt unavailable after Worker restart')
assert subprocess.run(['sudo', '-n', 'test', '-e', str(base / 'quarantine' / 'synthetic.txt')]).returncode == 1
assert hashlib.sha256((base / 'sentinel').read_bytes()).hexdigest() == (base / 'sentinel.sha').read_text().split()[0]
def source_request(op, expected=200, **fields):
    return request({'op': 'SOURCE_DIRECT_PURGE_' + op, **fields}, expected, '/gov/source-direct-purge')

def source_plan(target, keeper, expected=200):
    return source_request('PLAN', expected, storage_id='synthetic-local', target_id=target, keeper_id=keeper)

def source_act(p, op, expected=200, **fields):
    return source_request(op, expected, plan_id=p['ID'], digest=p['Digest'], **fields)

source_plan(source_one, source_one, 409)
p1 = source_plan(source_one, keeper_one)
p2 = source_plan(source_two, keeper_two)
source_act(p1, 'EXECUTE', 409, confirmation='永久删除 ' + p1['ID'])
source_act(p1, 'APPROVE')
source_act(p1, 'DRY_RUN')
source_act(p2, 'APPROVE')
source_act(p2, 'DRY_RUN')
source_act(p2, 'EXECUTE', 409, confirmation='永久删除 ' + p1['ID'])
assert source_act(p1, 'EXECUTE', confirmation='永久删除 ' + p1['ID'])['State'] == 'PURGED'
assert source_act(p2, 'EXECUTE', confirmation='永久删除 ' + p2['ID'])['State'] == 'PURGED'
for p in (p1, p2):
    assert source_act(p, 'RECEIPT')['State'] == 'PURGED'
    source_act(p, 'EXECUTE', 409, confirmation='永久删除 ' + p['ID'])
for name in ('one', 'two'):
    assert subprocess.run(['sudo', '-n', 'test', '-e', str(base / 'source' / (name + '-remove.txt'))]).returncode == 1
    keeper = subprocess.check_output(['sudo', '-n', 'cat', str(base / 'source' / (name + '-keeper.txt'))])
    assert keeper == ('synthetic source group ' + name + '\n').encode()
assert hashlib.sha256((base / 'sentinel').read_bytes()).hexdigest() == (base / 'sentinel.sha').read_text().split()[0]
print(json.dumps({'synthetic_quarantine_file_deleted': True, 'synthetic_source_files_deleted': 2,
                  'verified_keepers': 2, 'independent_approval_and_confirmation': True, 'plan_and_dry_run_required': True,
                  'tls_worker_receipt_verified_after_restart': True, 'outside_sentinel_unchanged': True}))
PY
web_id="$("${compose[@]}" ps -q web)"
worker_id="$("${compose[@]}" ps -q worker)"
test "$(docker inspect --format '{{json .Mounts}}' "$web_id" | python3 -c 'import json,sys; print(sorted(m["Destination"] for m in json.load(sys.stdin)))')" = "['/config', '/run/ndg-worker']"
test "$(docker inspect --format '{{.HostConfig.NetworkMode}} {{.Config.User}}' "$worker_id")" = 'none 10002:10002'
test "$(docker inspect --format '{{.Config.User}}' "$web_id")" = '10001:10001'
test -z "$(docker port "$web_id")"
