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
for line in Path('browser-flow.log').read_text().splitlines():
    if re.match(r'^(Error|TimeoutError):', line):
        text = re.sub(r'ndg-gov-[0-9a-fA-F]+', '[fixture credential]', line)
        print(text[:250])
PY
  exit 1
}
test "$(sudo -n test -e source/temp/one-remove.pdf && printf present || printf absent)" = absent
test "$(sudo -n test -e quarantine/synthetic.txt && printf present || printf absent)" = absent
sudo -n test -f source/temp/one-keeper.pdf
sudo -n test -f source/temp/two-remove.pdf
sha256sum -c sentinel.sha >&2
printf '%s\n' '{"real_chrome":true,"source_browser_purge":true,"quarantine_browser_purge":true,"independent_plan_cancellation":true,"receipt_reload_verified":true,"outside_sentinel_unchanged":true}'
