#!/usr/bin/env bash
set -euo pipefail
[[ "${GITHUB_ACTIONS:-}" == true && "${RUNNER_ENVIRONMENT:-}" == github-hosted ]]
fixture="$1"
[[ "$fixture" == /tmp/ndg-quarantine-ci-* && -d "$fixture" ]]
umask 077
scenario="$(pwd)/scripts/qa/purge-console-browser.js"
cd "$fixture"
pw=(npx --yes --package @playwright/cli@0.1.25 playwright-cli --session ndg-purge)
trap '"${pw[@]}" close >/dev/null 2>&1 || true' EXIT
spki="$(openssl x509 -in client.crt -pubkey -noout | openssl pkey -pubin -outform der | openssl dgst -sha256 -binary | openssl base64 -A)"
python3 - "$spki" "$scenario" <<'PY'
import json, sys
from pathlib import Path
Path('browser-config.json').write_text(json.dumps({
    'browser': {'launchOptions': {'args': ['--ignore-certificate-errors-spki-list=' + sys.argv[1]]}},
}))
scenario = Path(sys.argv[2]).read_text()
args = [Path('client-token').read_text().strip(), Path('item').read_text().strip()]
for phase in ('prepare-source', 'confirm-source', 'prepare-quarantine', 'verify-reload'):
    Path(phase + '.js').write_text('async page => (' + scenario + ')(page, ...' + json.dumps([*args, phase]) + ')')
PY
"${pw[@]}" open about:blank --browser chrome --config browser-config.json >browser-open.log 2>&1 || {
  printf 'Browser startup failed\n' >&2
  exit 1
}
run_phase() {
  if "${pw[@]}" run-code --filename "$1.js" >browser-flow.log 2>&1; then return; fi
  printf 'Browser phase %s failed\n' "$1" >&2
  # Do not echo the CLI transcript, which contains the fixture credential.
  python3 - <<'PY'
import re
from pathlib import Path
lines = Path('browser-flow.log').read_text().splitlines()
if '### Error' in lines:
    start = lines.index('### Error') + 1
    for line in lines[start:start + 10]:
        if line.startswith('### '):
            break
        print(re.sub(r'ndg-gov-[0-9a-fA-F]+', '[fixture credential]', line)[:250])
else:
    print('Playwright scenario failed before reporting a browser error')
PY
  return 1
}
confirm_prompt() {
  local phrase
  phrase="$(python3 - "$1" <<'PYCODE'
import re
import sys
from pathlib import Path
log = Path('browser-flow.log').read_text()
matches = re.findall(r'^' + sys.argv[1] + r' [a-f0-9]{64}(?="\])', log, re.MULTILINE)
if '### Modal state' not in log or len(matches) != 1:
    raise SystemExit('Expected exactly one confirmation dialog')
print(matches[0])
PYCODE
)"
  "${pw[@]}" dialog-accept "$phrase" >/dev/null
}
run_phase prepare-source
"${pw[@]}" dialog-accept '错误确认' >/dev/null
run_phase confirm-source
confirm_prompt 永久删除
run_phase prepare-quarantine
confirm_prompt 永久清理
run_phase verify-reload
grep -Fx '"NDG browser verification complete"' browser-flow.log >/dev/null

assert_fixture_file() {
  local expected="$1" file="$2" actual
  if sudo -n test -f "$file"; then actual=present; else actual=absent; fi
  if [[ "$actual" != "$expected" ]]; then
    printf 'Fixture file %s expected %s but was %s\n' "$file" "$expected" "$actual" >&2
    return 1
  fi
}
assert_fixture_file absent source/temp/one-remove.pdf
assert_fixture_file absent quarantine/synthetic.txt
assert_fixture_file present source/temp/one-keeper.pdf
assert_fixture_file present source/temp/two-remove.pdf
sha256sum -c sentinel.sha >&2
printf '%s\n' '{"real_chrome":true,"source_browser_purge":true,"quarantine_browser_purge":true,"independent_plan_cancellation":true,"receipt_reload_verified":true,"outside_sentinel_unchanged":true}'
