#!/bin/sh
# No FPK is produced by this research wrapper. No daemon/tool download is invoked.
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec python3 "$script_dir/check-fnos-package.py" --release "$@"
