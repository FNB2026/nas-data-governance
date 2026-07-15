# Third-Party Notices

The `nas-governance` command uses the following runtime Go modules. This inventory was generated from `go list -deps` for `./cmd/nas-governance` and must be reviewed whenever `go.mod` or `go.sum` changes.

| Module | Version | License |
|---|---:|---|
| `github.com/dslipak/pdf` | `v0.0.2` | BSD-3-Clause |
| `github.com/dustin/go-humanize` | `v1.0.1` | MIT |
| `github.com/google/uuid` | `v1.6.0` | BSD-3-Clause |
| `github.com/mattn/go-isatty` | `v0.0.20` | MIT |
| `github.com/ncruces/go-strftime` | `v1.0.0` | MIT |
| `github.com/remyoudompheng/bigfft` | `v0.0.0-20230129092748-24d4a6f8daec` | BSD-3-Clause |
| `golang.org/x/sys` | `v0.44.0` | BSD-3-Clause |
| `golang.org/x/text` | `v0.28.0` | BSD-3-Clause |
| `modernc.org/libc` | `v1.73.4` | BSD-3-Clause plus bundled third-party notices |
| `modernc.org/mathutil` | `v1.7.1` | BSD-3-Clause |
| `modernc.org/memory` | `v1.11.0` | BSD-3-Clause plus bundled component licenses |
| `modernc.org/sqlite` | `v1.53.0` | BSD-3-Clause |

`make release` runs `scripts/collect-third-party-licenses.sh` and includes the verbatim upstream `LICENSE*`, `COPYING*`, and `NOTICE*` files discovered for these runtime modules in each release archive. This notice is an inventory, not a substitute for those license texts.

If the runtime dependency graph changes, update this table and verify the collected license bundle before publishing a release.
