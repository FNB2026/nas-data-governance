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
Path('browser-flow.js').write_text('async page => (' + scenario + ')(page, ...' + json.dumps(args) + ')')
PY
"${pw[@]}" open about:blank --browser chrome --config browser-config.json >browser-open.log 2>&1 || {
  printf 'Browser startup failed\n' >&2
  exit 1
}
"${pw[@]}" run-code --filename browser-flow.js >browser-flow.log 2>&1 || {
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
  exit 1
}
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
