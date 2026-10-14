# Offline own-container namespace HTTP probe prototype

Status: **BLOCKED for execution**. This change supplies a pure constraint model,
executable refusal checks and a proposed native CI capability test. It contains
no Docker client, namespace syscall, privilege transition, child launcher,
credential reader, HTTP client, resource deletion or live backend. The model's
`OFFLINE_MODEL_ACCEPTED` means synthetic assertions satisfy its constraints;
it cannot establish kernel behavior, resource ownership or successful HTTP.

## Coverage boundaries

| Evidence | This change |
| --- | --- |
| Default invocation and explicit prototype refusal | Executable offline CLI checks |
| Witness, request, privilege and cleanup constraints | Pure synthetic model tests |
| Namespace FD pinning, PID reuse prevention, child privilege drop | NOT RUN; no implementation |
| Actual own-container numeric loopback HTTP | NOT RUN |
| Host loopback published port HTTP | NOT RUN; a separate transport |
| fnOS Docker 28.5.2 historical trial | Historical LIMITED BLOCKED remains unchanged |
| NAS/SSH/SMB/source writes/production Docker | NOT AUTHORIZED by this prototype |

The frozen scratch image has `/ndg-server` and licenses, with no shell or HTTP
toolbox. A future host helper must join only its exact owned container's network
namespace. A successful request there cannot turn a host published-port refusal
into PASS. The existing host smoke script, SDK and product APIs are unchanged.
Historical evidence and beta.11 assets are unchanged.

## Executable surface

```sh
python3 -B scripts/packaging/nas-namespace-probe.py
# Exit 0: protocol_only_no_io, live_backend BLOCKED
python3 -B scripts/packaging/nas-namespace-probe.py --prototype
# Exit 2: live_backend_requires_independent_review
make nas-namespace-model-test
```

Both CLI modes print fixed sanitized JSON and perform no application Docker,
file, source, namespace or network I/O. Normal interpreter/module loading is
outside that claim. No PID, namespace path, URL, shell command, credential or
output-directory flags exist. The CLI cannot accept/replay supplied witnesses.
`replay` is an in-memory test function with no callbacks or executable adapter.
Rejected argument messages use a fixed JSON classification. Actual CLI tests
use a fresh random synthetic marker in unknown option values, positional input,
unknown `--option=value` and invalid `--prototype=value`; all four exit 2 with
empty stdout and `unsupported_arguments` stderr, without echoing the marker.

The synthetic model compares exact daemon/container/network IDs, nonce, frozen
image digest, boot identity, State.Pid, starttime and network namespace device /
inode before pinning, before launching the child and after HTTP. It requires
pidfd liveness, exactly one owned network attachment, internal isolated IPv4,
disabled IPv6, no published ports and a namespace different from the host.
All are assertions supplied by tests; none are collected from a host.

The tool model requires canonical `/usr/bin/nsenter`, a pinned digest, root
ownership, trusted ancestors, a regular executable with no set-ID, other-user
write permissions or file capabilities. This is a proposed trust policy, not
verification of a local executable. It does not resolve the interpreter,
dynamic loader and helper provenance chain; those remain activation blockers.

Ordered model phases are snapshot, pin, ownership recheck, child-only namespace
transition, privilege readback, fixed request, bounded response, ownership
postcheck and reap. The request is exactly one `GET /healthz` to numeric
`127.0.0.1:8080`, with no DNS, proxy, redirect, shell or arbitrary request path.
Credentials are represented only by a bounded exclusive private FIFO witness,
not secret bytes. Child argv is fixed, its environment is empty and only the
logical namespace/credential handles are inherited. Model limits are 10 seconds,
64 MiB memory, 16 FDs and 4096 response/output bytes. Synthetic privilege readback
requires real/effective/saved UID and GID 65534, no supplementary groups, all five
capability sets zero and no-new-privileges true before a request.

On partial protocol, timeout or failure after acquiring a child handle, the
model retains kill/wait obligations through that owned logical pidfd and close
obligations for every acquired logical descriptor. Those intents execute
nothing and are not actual FD/child cleanup evidence. No output/resource delete
intent exists. A future backend must retain exact private evidence on failure,
refuse foreign or partial output cleanup, and independently verify actual
descriptor ownership and child termination.

## Why the transport needs a separate gate

[Docker's bridge documentation](https://docs.docker.com/engine/network/drivers/bridge/)
describes host access to bridge containers. [The gateway-mode documentation](https://docs.docker.com/engine/network/port-publishing/)
explains that internal networks ordinarily have a bridge address and can reach
host services bound there; `isolated` removes that bridge address. Therefore
`--internal` alone is not evidence of a host air gap. Both address families,
actual routes, network attachments and host reachability need explicit testing.

[Moby v28.5.2 endpoint.go](https://github.com/moby/moby/blob/v28.5.2/libnetwork/endpoint.go)
uses `!n.internal` as the condition for the external-connectivity branch in
`sbJoin`; this does not describe every helper call to
`ProgramExternalConnectivity`. That branch is consistent with
the observed pre-HTTP publish refusal, but does not prove all Docker versions or
configurations behave identically. [setns(2)](https://man7.org/linux/man-pages/man2/setns.2.html)
changes the calling thread's namespace; a helper must explicitly select
`CLONE_NEWNET`. A multithreaded Go HTTP helper without OS-thread ownership would
need additional proof. [capabilities(7)](https://man7.org/linux/man-pages/man7/capabilities.7.html)
and [PR_SET_NO_NEW_PRIVS(2)](https://man7.org/linux/man-pages/man2/PR_SET_NO_NEW_PRIVS.2const.html)
are relevant to privilege readback; no-new-privileges alone does not drop caps.

## Feasible next native CI milestone, pending exact independent review

The existing `linux-web.yml` disposable-container job already has native Ubuntu
amd64/arm64 runners, sudo and locally built scratch containers. It is a suitable
execution environment for a **separate** capability job after review. Ubuntu
availability does not prove the required Python 3.11 toolchain, nsenter trust,
kernel capabilities or Docker `isolated` support. No privileged CI job is added
or enabled in this change. Ordinary CI runs only the offline model tests.

The smallest first real test should not contact Docker or the product server:

1. Compile a small reviewed fixed Linux syscall bridge from exact checked-out
   source on a native ephemeral runner. Record source/binary digest and compiler
   identity. No arbitrary command, PID, namespace path or URL options.
2. In a child only, create a disposable `CLONE_NEWNET` namespace, activate its
   loopback and serve a bounded synthetic HTTP marker at numeric loopback:8080.
   The supervisor never enters any namespace. Pin the owned child with pidfd
   and namespace FD, checking boot/starttime and namespace type/identity around
   acquisition. This tests primitives; it is not Docker ownership acceptance.
3. Spawn a second fixed child, join only the pinned network namespace, close its
   namespace descriptor, drop capability bounding/ambient sets and groups/GID/
   UID in reviewed order, clear remaining capabilities, set no-new-privileges,
   and read back all privilege sets before its single fixed request. Test forced
   transition/readback failures and refuse any HTTP on failure.
4. Retain no source/state mounts, host port publication, registry pulls, image
   publication, NAS transport or production daemon configuration. Bound FD,
   memory, CPU, wall time, requests and output independently of server behavior.
   Verify raw-socket/privileged operations fail after the drop. Exercise PID
   exit/reuse, timeout, private-FD transfer/redaction and kernel kill/reap/close.
5. Only after those primitives pass, review a Docker binding layer that records
   exact fresh daemon/container/network/image/nonce ownership; creates one own
   internal isolated network and scratch container; and repeats identity checks
   around pinning/child/request/postcheck. Reject all foreign attachments,
   symlink/mount uncertainty and unconfirmed toolchains. No automatic deletion
   of unconfirmed resources. This layer also needs source read-only/unchanged
   byte evidence and full HTTP acceptance independently from host publication.

Activation requires independent review of that exact bridge, test fixture and
workflow diff, successful kernel evidence on each native architecture, and a
fresh operator-bound own-container transport record before any live trial.
This design can progress on ephemeral CI; macOS NOT RUN is not a permanent
technical inability. These prerequisites remain concrete outstanding work.

## Offline acceptance matrix

The standalone tests cover changed/wrong PID and reused starttime; changed
container, daemon, nonce, boot, namespace and tool identity; frozen image and
network isolation refusal; missing/mutable/untrusted tool; extra namespace or
master switch; incomplete privilege readback and limits; URL, DNS, redirect,
proxy and command injection; private FD exclusivity/size/inheritance; redaction;
timeout/output/HTTP refusal; and interrupted-model cleanup obligations. They
are pure mocks, except CLI exit/output checks. Actual Linux kernel behavior is
NOT RUN on the development macOS host, and no test here claims otherwise.

Local verification on macOS for this change: `make test`, `make vet`,
`go test -race -count=1 ./...`, `make build`, `make public-check` and
`git diff --check` passed. The standalone namespace model has 14 passing test
methods; existing `nas-smoke-test` has 39 and `container-evidence-test` has 19.
These are local results, not GitHub CI or device acceptance. Complete local
outputs and the exact commit/tree/diff/file digests are retained privately for
the independent review handoff rather than committed as runtime evidence.
