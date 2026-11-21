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
mkdir -m 0700 "$fixture"/{quarantine,source}/.ndg-purge-stage "$fixture/source/temp"
printf 'stage metadata stays outside the scanner index\n' > "$fixture/source/.ndg-purge-stage/synthetic-marker"
for group in one two stale; do
  for role in remove keeper; do
    printf '%%PDF-1.7\nsynthetic source group %s\n' "$group" > "$fixture/source/temp/${group}-${role}.pdf"
  done
done
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
: "${NDG_PURGE_CLI:?build candidate CLI first}"
# Run the shipping scanner and analyzer against the same path the Worker
# will open. The ingestion process sees the source volume read-only.
: "${NDG_SCAN_IMAGE:?build native scanner image first}"
scan_tool=(docker run --rm --network none --read-only --cap-drop ALL
  --security-opt no-new-privileges --user 10002:10002
  --mount "type=bind,src=$fixture/source,dst=/ndg-volume,readonly"
  --mount "type=bind,src=$fixture/state,dst=/state" "$NDG_SCAN_IMAGE")
"${scan_tool[@]}" scan --root /ndg-volume --storage candidate-volume --db /state/project.db --out /state/index.jsonl >&2
"${scan_tool[@]}" analyze --confined-root /ndg-volume --index /state/index.jsonl --storage-id candidate-volume --db /state/project.db --out /state/formats.json >&2
"${compose[@]}" up -d --no-build --pull never >/dev/null
if [[ "${NDG_TEST_PURGE_BROWSER:-}" == 1 ]]; then
  bash scripts/qa/test-purge-console-browser.sh "$fixture"
  exit
fi
python3 -B - "$fixture" "${compose[@]}" <<'PY'
import hashlib
import json
from pathlib import Path
import os
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

duplicate_worker = subprocess.run(sys.argv[2:] + ['run', '--no-deps', '--rm', 'worker'], capture_output=True, timeout=20)
assert duplicate_worker.returncode != 0

with opener.open(url + '/gov/purge-console', timeout=5) as page:
    assert "frame-ancestors 'none'" in page.headers.get('Content-Security-Policy', '')
    assert b'id="files"' in page.read()
with opener.open(url + '/gov/purge-console.js', timeout=5) as js:
    assert js.headers.get_content_type() == 'text/javascript'
    assert b'SOURCE_DIRECT_PURGE_LIST' in js.read()
token = (base / 'client-token').read_text().strip()
item_id = (base / 'item').read_text().strip()
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
subprocess.run(sys.argv[2:] + ['kill', '-s', 'SIGKILL', 'worker'], check=True, capture_output=True)
subprocess.run(sys.argv[2:] + ['start', 'worker'], check=True, capture_output=True)
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
    return source_request('PLAN', expected, storage_id='candidate-volume', target_id=target, keeper_id=keeper)

def source_act(p, op, expected=200, **fields):
    return source_request(op, expected, plan_id=p['id'], digest=p['digest'], **fields)

choices = source_request('LIST')['choices']
cli = [os.environ['NDG_PURGE_CLI'], '-ca-file', str(base / 'client.crt'), '-token-file', str(base / 'client-token')]
assert json.loads(subprocess.check_output(cli + ['-action', 'list']))['choices'] == choices
ids = {c['path']: c['file_id'] for c in choices}
assert set(ids) == {f'/ndg-volume/temp/{g}-{r}.pdf' for g in ('one', 'two', 'stale') for r in ('remove', 'keeper')}
assert len(set(ids.values())) == 6
def file_id(group, role):
    return ids[f'/ndg-volume/temp/{group}-{role}.pdf']
source_one, keeper_one = file_id('one', 'remove'), file_id('one', 'keeper')
source_two, keeper_two = file_id('two', 'remove'), file_id('two', 'keeper')
source_stale, keeper_stale = file_id('stale', 'remove'), file_id('stale', 'keeper')
source_plan(source_one, source_one, 409)
unknown_plan = '0' * 64
source_request('EXECUTE', 409, plan_id=unknown_plan, digest='e' * 64, confirmation='synthetic refused input')
p1 = source_plan(source_one, keeper_one)
p2 = source_plan(source_two, keeper_two)
p3 = source_plan(source_stale, keeper_stale)
source_act(p1, 'EXECUTE', 409, confirmation='永久删除 ' + p1['id'])
source_act(p1, 'APPROVE')
source_act(p1, 'DRY_RUN')
source_act(p2, 'APPROVE')
source_act(p2, 'DRY_RUN')
source_act(p2, 'EXECUTE', 409, confirmation='永久删除 ' + p1['id'])
source_act(p3, 'APPROVE')
source_act(p3, 'DRY_RUN')
stale_path = str(base / 'source' / 'temp' / 'stale-remove.pdf')
subprocess.run(['sudo', '-n', 'tee', stale_path], input=b'changed after dry run', stdout=subprocess.DEVNULL, check=True)
source_act(p3, 'EXECUTE', 409, confirmation='永久删除 ' + p3['id'])
assert source_act(p3, 'RECEIPT')['state'] == 'DRY_RUN'
assert subprocess.check_output(['sudo', '-n', 'cat', stale_path]) == b'changed after dry run'
assert source_act(p1, 'EXECUTE', confirmation='永久删除 ' + p1['id'])['state'] == 'PURGED'
assert source_act(p2, 'EXECUTE', confirmation='永久删除 ' + p2['id'])['state'] == 'PURGED'
for p in (p1, p2):
    assert source_act(p, 'RECEIPT')['state'] == 'PURGED'
    source_act(p, 'EXECUTE', 409, confirmation='永久删除 ' + p['id'])
for name in ('one', 'two'):
    assert subprocess.run(['sudo', '-n', 'test', '-e', str(base / 'source' / 'temp' / (name + '-remove.pdf'))]).returncode == 1
    keeper = subprocess.check_output(['sudo', '-n', 'cat', str(base / 'source' / 'temp' / (name + '-keeper.pdf'))])
    assert keeper == ('%PDF-1.7\nsynthetic source group ' + name + '\n').encode()
assert hashlib.sha256((base / 'sentinel').read_bytes()).hexdigest() == (base / 'sentinel.sha').read_text().split()[0]
def readonly_query(query, *args):
    python = ("import json,sqlite3,sys; db=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True); "
              "print(json.dumps(db.execute(sys.argv[2],sys.argv[3:]).fetchall()))")
    return json.loads(subprocess.check_output(['sudo','-n','python3','-c',python,str(base/'state'/'project.db'),
                                               query,*args], text=True))
assert readonly_query("SELECT kind,root_path FROM storages WHERE id=?", "candidate-volume") == [["filesystem", "/ndg-volume"]]
assert readonly_query("SELECT status FROM scan_checkpoints WHERE storage_id=?", "candidate-volume") == [["completed"]]
assert readonly_query("SELECT COUNT(*) FROM file_instances WHERE storage_id=?", "candidate-volume") == [[6]]
assert readonly_query("SELECT COUNT(*) FROM file_formats JOIN file_instances ON file_id=file_instances.id WHERE storage_id=?", "candidate-volume") == [[6]]
assert readonly_query("SELECT file_status FROM file_instances WHERE id=?", source_one) == [["missing"]]
assert readonly_query("SELECT file_status FROM file_instances WHERE id=?", keeper_one) == [["active"]]
def audit_states(purge_plan):
    # Read the Worker-owned synthetic SQLite database as root on this
    # disposable runner. Inspect only stable codes and opaque plan IDs.
    query = ("SELECT detail FROM authorization_audit WHERE plan_id=? AND eligible=1 ORDER BY id")
    return ','.join(row[0] for row in readonly_query(query, purge_plan['id']))
expected = 'DRAFT,APPROVED,DRY_RUN,EXECUTING,PURGED'
for p in (plan, p1, p2):
    assert audit_states(p) == expected
assert audit_states(p3) == 'DRAFT,APPROVED,DRY_RUN'
refused = "SELECT operation,reason,item_id,detail FROM authorization_audit WHERE plan_id=? AND eligible=0 ORDER BY id"
assert readonly_query(refused, plan['id']) == [["QUARANTINE_PURGE_EXECUTE", "REFUSED", "", ""]]
assert readonly_query(refused, p3['id']) == [["SOURCE_DIRECT_PURGE_EXECUTE", "REFUSED", "", ""]]
assert len(readonly_query(refused, p2['id'])) == 2
assert not readonly_query(refused, unknown_plan)
assert readonly_query(refused, "")[-1] == ["SOURCE_DIRECT_PURGE_EXECUTE", "REFUSED", "", ""]
for p, mode in ((plan, 'quarantine'), (p1, 'source')):
    receipt = subprocess.check_output(cli + ['-mode', mode, '-action', 'receipt', '-plan-id', p['id'], '-digest', p['digest']])
    assert json.loads(receipt)['state'] == 'PURGED'
print(json.dumps({'synthetic_quarantine_file_deleted': True, 'synthetic_source_files_deleted': 2,
                  'native_scan_analyze_project_verified': True, 'verified_keepers': 2, 'independent_approval_and_confirmation': True,
                  'stale_selected_file_preserved': True, 'durable_audit_states_verified': True, 'durable_refusal_audit_verified': True, 'tls_cli_receipts_verified': True, 'plan_and_dry_run_required': True,
                  'tls_worker_receipt_verified_after_restart': True, 'worker_sigkill_stale_socket_recovered': True, 'duplicate_worker_refused': True, 'outside_sentinel_unchanged': True}))
PY
web_id="$("${compose[@]}" ps -q web)"
worker_id="$("${compose[@]}" ps -q worker)"
test "$(docker inspect --format '{{json .Mounts}}' "$web_id" | python3 -c 'import json,sys; print(sorted(m["Destination"] for m in json.load(sys.stdin)))')" = "['/config', '/run/ndg-worker']"
test "$(docker inspect --format '{{.HostConfig.NetworkMode}} {{.Config.User}}' "$worker_id")" = 'none 10002:10002'
test "$(docker inspect --format '{{.Config.User}}' "$web_id")" = '10001:10001'
test -z "$(docker port "$web_id")"
