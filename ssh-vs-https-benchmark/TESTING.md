# Testing Strategy

Testing plan for the SSH vs HTTPS CLI transport benchmark tool.
Focuses on gaps that matter for benchmark accuracy and correctness.

---

## 1. Current Test Coverage Assessment

### What's Tested

| Package | File | Coverage |
|---|---|---|
| `internal/device` | `device_test.go` | Exec exact/prefix/ambiguous/unknown/empty, hostname substitution, `Commands()`, `prefixMatch` |
| `internal/latency` | `latency_test.go` | Direction-change delay, consecutive same-direction no-delay, zero-delay passthrough |
| `internal/integration_test.go` | (package `integration_test`) | SSH exec, SSH bad auth, HTTPS exec, HTTPS bad auth, HTTPS multi-cmd GET, HTTPS config POST |

### What's NOT Tested — ~~Gaps~~ Resolved

All gaps identified below have been addressed. Tests are in place.

| Package | Former Gap | Resolution |
|---|---|---|
| `internal/sshserver` | No unit tests | `server_test.go`: multi-session, batch payload, unknown command, port-in-use |
| `internal/httpserver` | No unit tests | `server_test.go`: empty path, wrong method, empty body, URL decoding, port-in-use |
| `internal/proxy` | Zero tests | `proxy_test.go`: fresh exec, pooled exec, pool reset, bad auth, backend down, config POST, backend dies mid-request |
| `internal/netem` | Zero tests | `netem_test.go` (build tag `netem_root`): setup/teardown, idempotent setup, teardown no-op. `netem_unpriv_test.go`: unprivileged error check |
| `internal/tlsutil` | Zero tests | `tlsutil_test.go`: cert validity |
| `cmd/bench` / `internal/stats` | `summarize`, `percentile`, `runParallel`, `generateExecPayload` untested | Extracted to `internal/stats`. `stats_test.go`: percentile, interpolation, known values, summarize basic/all-errors/partial-errors, stddev, errDuration sentinel, generate payload, runParallel concurrency/errors |

### ~~Critical Gaps~~ Resolved

1. ~~**No validation that netem actually adds delay.**~~ → `netem_test.go` (root-gated) validates setup/teardown via `netlink.QdiscList`.
2. ~~**No test that SSH and HTTPS return identical output.**~~ → `TestBackendEquivalence`, `TestSSHAndHTTPSReturnIdenticalOutput`, `TestSSHAndHTTPSBatchIdenticalOutput`, `TestProxyReturnsIdenticalOutput`.
3. ~~**`summarize()` statistics are untested.**~~ → Extracted to `internal/stats`, fully tested including sample stddev (Bessel's correction).
4. ~~**Error counting (`errDuration` sentinel) is untested.**~~ → `TestErrDurationSentinel`, `TestSummarizeAllErrors`, `TestSummarizePartialErrors`.
5. ~~**Proxy pool reset logic is untested.**~~ → `TestProxyPooledResetOnError`, `TestProxyBackendDiesMidRequest` (uses TCP proxy to reliably kill connections).

---

## 2. Unit Test Plan

### `internal/stats` (extracted from `cmd/bench`)

| Test | Validates | Root | Priority |
|---|---|---|---|
| `TestPercentile` | Percentile calculation: p0, p50, p95, p100, single element, empty slice | No | P0 |
| `TestPercentileInterpolation` | Fractional rank interpolation between adjacent values | No | P0 |
| `TestSummarizeBasic` | Avg, min, max, stddev for known input durations | No | P0 |
| `TestSummarizeAllErrors` | All iterations return `errDuration` → errors == iterations, zero stats | No | P0 |
| `TestSummarizePartialErrors` | Mix of valid and `errDuration` → correct error count, stats from valid only | No | P0 |
| `TestGenerateExecPayload` | N commands → N newline-delimited `show version\n` lines | No | P1 |
| `TestRunParallelConcurrency` | With concurrency=1, iterations execute serially (no overlap) | No | P1 |

### `internal/sshserver`

| Test | Validates | Root | Priority |
|---|---|---|---|
| `TestSSHExecMultiSession` | Multiple sequential sessions on one connection return correct output | No | P0 |
| `TestSSHExecUnknownCommand` | Unknown command returns `%` error prefix, not empty | No | P1 |
| `TestSSHExecBatchPayload` | Multi-line exec payload (newline-separated) returns concatenated output | No | P0 |
| `TestSSHBadChannelType` | Non-"session" channel type is rejected | No | P2 |

### `internal/httpserver`

| Test | Validates | Root | Priority |
|---|---|---|---|
| `TestHTTPSExecEmptyPath` | `GET /admin/exec/` returns 400 | No | P1 |
| `TestHTTPSConfigGetMethod` | `GET /admin/config` returns 405 | No | P1 |
| `TestHTTPSConfigEmptyBody` | `POST /admin/config` with empty body returns 200 with empty output | No | P1 |
| `TestHTTPSExecURLDecoding` | `show+ip+interface+brief` decodes to `show ip interface brief` | No | P1 |

### `internal/proxy`

| Test | Validates | Root | Priority |
|---|---|---|---|
| `TestProxyFreshExec` | Fresh mode: each request dials a new SSH connection, returns correct output | No | P0 |
| `TestProxyPooledExec` | Pooled mode: multiple requests reuse one SSH connection | No | P0 |
| `TestProxyPooledResetOnError` | After backend SSH goes down, pool resets and next request reconnects | No | P0 |
| `TestProxyBadAuth` | Wrong credentials return 401 | No | P1 |
| `TestProxyBackendDown` | Backend SSH unreachable → 502 Bad Gateway | No | P1 |
| `TestProxyConfigPost` | POST `/admin/config` with multi-line body returns concatenated output | No | P1 |

### `internal/tlsutil`

| Test | Validates | Root | Priority |
|---|---|---|---|
| `TestSelfSignedConfigValid` | Returns non-nil config with one certificate, cert is valid for current time | No | P1 |
| `TestSelfSignedConfigUnique` | Two calls produce different certificates (fresh key each time) | No | P2 |

### `internal/netem`

| Test | Validates | Root | Priority |
|---|---|---|---|
| `TestSetupTeardown` | Setup creates prio qdisc on loopback, Teardown removes it. Verify via `netlink.QdiscList`. | **Yes** | P0 |
| `TestSetupIdempotent` | Calling Setup twice doesn't error (Teardown is called internally first) | **Yes** | P1 |
| `TestTeardownNoOp` | Teardown on clean interface doesn't panic or error | **Yes** | P1 |

### `internal/device`

Existing tests are solid. One gap:

| Test | Validates | Root | Priority |
|---|---|---|---|
| `TestNewMissingDir` | `New()` with nonexistent transcript dir returns error | No | P1 |
| `TestNewEmptyDir` | `New()` with empty dir succeeds with zero commands | No | P1 |
| `TestNewNonTxtFilesIgnored` | `.json`, `.md` files in transcript dir are skipped | No | P2 |

### `internal/latency`

Existing tests cover direction-change and zero-delay. One gap:

| Test | Validates | Root | Priority |
|---|---|---|---|
| `TestListenerNegativeDelay` | Negative delay treated same as zero (no wrap) | No | P2 |

---

## 3. Integration Test Plan

All integration tests live in `internal/integration_test.go` (or a new `internal/integration_proxy_test.go`). None require root.

| Test | Validates | Priority |
|---|---|---|
| `TestSSHAndHTTPSReturnIdenticalOutput` | Same command via SSH exec and HTTPS GET produces byte-identical output. This is the core "same backend" guarantee. | P0 |
| `TestSSHAndHTTPSBatchIdenticalOutput` | Multi-command batch via SSH exec (newline payload) and HTTPS POST `/admin/config` produces identical output. | P0 |
| `TestProxyReturnsIdenticalOutput` | Same command via direct SSH, direct HTTPS, and proxy HTTPS all return identical output. | P0 |
| `TestSSHMultipleSessionsOnOneConn` | Open connection, run 10 sequential sessions, all succeed. Validates the `reuse-conn` benchmark mode. | P1 |
| `TestHTTPSKeepAlive` | Single `http.Client` with keep-alives, 10 sequential requests, all succeed. Validates the `keep-alive` benchmark mode. | P1 |
| `TestHTTPSFreshConnDisableKeepAlive` | Client with `DisableKeepAlives: true`, multiple requests all succeed. Validates `fresh-conn` mode. | P1 |
| `TestConcurrentSSHSessions` | 10 goroutines each open a session on the same connection simultaneously. | P1 |
| `TestConcurrentHTTPSRequests` | 10 goroutines each make HTTPS requests simultaneously. | P1 |
| `TestProxyPooledConcurrent` | 10 concurrent requests through pooled proxy all succeed. | P1 |

---

## 4. Benchmark Validation Tests

These tests verify the benchmark measures what it claims. They're the most important category for a measurement tool.

### 4a. Latency Injection Validation

| Test | Package | Validates | Root | Priority |
|---|---|---|---|---|
| `TestNetemAddsDelay` | `internal/netem` | Setup with 50ms delay on a port, then TCP connect + round-trip on that port takes ≥ 50ms. Teardown, same operation takes < 5ms. | **Yes** | P0 |
| `TestNetemPortIsolation` | `internal/netem` | Delay on port A does not affect port B. TCP round-trip on port B stays < 5ms. | **Yes** | P0 |
| `TestUserspaceDelayAddsLatency` | `internal/latency` | SSH exec through a latency-wrapped listener with 50ms delay takes ≥ 50ms. Without wrapper, takes < 10ms. | No | P0 |
| `TestNetemCampusVsWanDelay` | `internal/netem` | WAN ports get WAN delay, campus ports get campus delay. Measured via TCP round-trip on each. | **Yes** | P1 |

### 4b. Backend Equivalence

| Test | Package | Validates | Root | Priority |
|---|---|---|---|---|
| `TestBackendEquivalence` | `integration_test` | For every command in `dev.Commands()`, SSH exec output == HTTPS GET output == HTTPS POST output. Byte-for-byte. | No | P0 |

### 4c. Error Counting

| Test | Package | Validates | Root | Priority |
|---|---|---|---|---|
| `TestErrDurationSentinel` | `internal/stats` | `summarize()` with known error durations produces correct `Errors` count and excludes errors from stats. | No | P0 |
| `TestRunParallelCountsErrors` | `internal/stats` | When `fn` returns `errDuration` for some iterations, the returned slice contains the sentinel values at the correct indices. | No | P1 |

### 4d. Statistical Accuracy

| Test | Package | Validates | Root | Priority |
|---|---|---|---|---|
| `TestPercentileKnownValues` | `internal/stats` | `percentile([10,20,30,40,50], 50)` == 30, `percentile([10,20,30,40,50], 95)` == 48. Verified against manual calculation. | No | P0 |
| `TestStddevKnownValues` | `internal/stats` | `summarize()` with `[2ms, 4ms, 4ms, 4ms, 5ms, 5ms, 7ms, 9ms]` produces sample stddev ≈ 2.14 (Bessel's correction). | No | P0 |

---

## 5. Edge Cases and Error Paths

| Scenario | Test | Package | Priority |
|---|---|---|---|
| Netem setup fails (no CAP_NET_ADMIN) | `TestNetemSetupUnprivileged` — call `Setup()` as non-root, verify it returns an error (not panic). | `internal/netem` | P0 |
| Port already in use | `TestSSHServerPortInUse` — listen on a port, then try `sshserver.ListenAndServe()` on same port, verify error. | `internal/sshserver` | P1 |
| Port already in use (HTTPS) | `TestHTTPSServerPortInUse` — same for httpserver. | `internal/httpserver` | P1 |
| Empty transcript directory | `TestDeviceNoTranscripts` — device with 0 commands, exec returns `% Unknown command` for everything. | `internal/device` | P1 |
| Transcript dir doesn't exist | `TestDeviceMissingDir` — `New()` returns error. | `internal/device` | P1 |
| SSH connection drops mid-session | `TestSSHServerConnDrop` — close the TCP conn from client side during exec, server doesn't panic. | `internal/sshserver` | P2 |
| Proxy backend SSH dies mid-request | `TestProxyBackendDiesMidRequest` — kill backend after pool established, verify 502 and pool reset. | `internal/proxy` | P1 |
| HTTPS request with no auth header | `TestHTTPSNoAuthHeader` — request without `Authorization` header returns 401. | `internal/httpserver` | P2 |
| Very large exec payload | `TestSSHExecLargePayload` — 1000-line exec payload, verify complete output returned. | `internal/sshserver` | P2 |
| Concurrent netem Setup/Teardown | `TestNetemConcurrentSetup` — two goroutines call Setup simultaneously, no panic (Teardown is called first internally). | `internal/netem` | P2 |

---

## 6. Test Infrastructure

### Build Tags and Test Organization

```
internal/
  device/device_test.go           # go test ./internal/device/
  device/device_extra_test.go     # missing dir, empty dir, no transcripts
  latency/latency_test.go         # go test ./internal/latency/
  netem/netem_test.go              # //go:build netem_root → sudo go test -tags netem_root ./internal/netem/
  netem/netem_unpriv_test.go       # unprivileged error check (no build tag)
  sshserver/server_test.go         # go test ./internal/sshserver/
  httpserver/server_test.go        # go test ./internal/httpserver/
  proxy/proxy_test.go              # go test ./internal/proxy/
  tlsutil/tlsutil_test.go          # go test ./internal/tlsutil/
  stats/stats_test.go              # go test ./internal/stats/
  integration_test.go              # existing
  integration_equiv_test.go        # backend equivalence, concurrent sessions, proxy integration
```

### Tag Scheme

| Tag | Requires | What it gates |
|---|---|---|
| (none) | Nothing | All unit tests, integration tests (SSH/HTTPS/proxy on localhost) |
| `netem_root` | `CAP_NET_ADMIN` / root | Tests that call `netem.Setup()` and verify actual kernel delay |
| `benchmark_validation` | Root + time-sensitive | End-to-end latency measurement validation tests |

### CI Considerations

- **Unprivileged CI (GitHub Actions, etc.):** `go test -race ./...` — runs everything except `netem_root` and `benchmark_validation` tagged tests.
- **Privileged CI (self-hosted runner with root):** `sudo go test -race -tags netem_root,benchmark_validation ./...`
- **Local dev:** `go test -race -count=1 ./...` for quick iteration.

### Test Helpers to Build

1. **`internal/testutil/servers.go`** — Extract `setupServers()` from `integration_test.go` into a shared helper that returns SSH addr, HTTPS addr, and a cleanup func. Reuse in proxy tests and benchmark validation tests.
2. **`internal/testutil/netem.go`** — Helper that skips test if not root: `func RequireRoot(t *testing.T) { if os.Getuid() != 0 { t.Skip("requires root") } }`

---

## 7. Recommended Test Commands

```bash
# All unprivileged tests (CI default)
go test -race -count=1 ./...

# Single package
go test -race -v ./internal/device/
go test -race -v ./internal/proxy/
go test -race -v ./internal/latency/

# Integration tests only
go test -race -v -run 'Test(SSH|HTTPS|Proxy)' ./internal/

# Netem tests (requires root)
sudo go test -race -v -tags netem_root ./internal/netem/

# Benchmark validation (requires root, time-sensitive)
sudo go test -race -v -tags netem_root,benchmark_validation -timeout 120s ./internal/

# Verbose with coverage
go test -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Just the stats/percentile tests (fast, no servers)
go test -race -v -run 'Test(Percentile|Summarize|GenerateExec)' ./internal/stats/

# Run only P0 tests (by convention, P0 tests don't have "P2" or "Nice" in name)
go test -race -v ./...
```

---

## 8. Priority Summary

### P0 — Must Have (benchmark correctness)

| # | Test | Package |
|---|---|---|
| 1 | `TestPercentile` | `internal/stats` |
| 2 | `TestPercentileInterpolation` | `internal/stats` |
| 3 | `TestSummarizeBasic` | `internal/stats` |
| 4 | `TestSummarizeAllErrors` | `internal/stats` |
| 5 | `TestSummarizePartialErrors` | `internal/stats` |
| 6 | `TestSSHExecMultiSession` | `internal/sshserver` |
| 7 | `TestSSHExecBatchPayload` | `internal/sshserver` |
| 8 | `TestProxyFreshExec` | `internal/proxy` |
| 9 | `TestProxyPooledExec` | `internal/proxy` |
| 10 | `TestProxyPooledResetOnError` | `internal/proxy` |
| 11 | `TestSetupTeardown` | `internal/netem` (root) |
| 12 | `TestNetemAddsDelay` | `internal/netem` (root) |
| 13 | `TestNetemPortIsolation` | `internal/netem` (root) |
| 14 | `TestUserspaceDelayAddsLatency` | `internal/latency` |
| 15 | `TestSSHAndHTTPSReturnIdenticalOutput` | `integration_test` |
| 16 | `TestSSHAndHTTPSBatchIdenticalOutput` | `integration_test` |
| 17 | `TestProxyReturnsIdenticalOutput` | `integration_test` |
| 18 | `TestBackendEquivalence` | `integration_test` |
| 19 | `TestErrDurationSentinel` | `internal/stats` |
| 20 | `TestPercentileKnownValues` | `internal/stats` |
| 21 | `TestStddevKnownValues` | `internal/stats` |
| 22 | `TestNetemSetupUnprivileged` | `internal/netem` |

### P1 — Should Have (robustness)

| # | Test | Package |
|---|---|---|
| 1 | `TestGenerateExecPayload` | `internal/stats` |
| 2 | `TestRunParallelConcurrency` | `internal/stats` |
| 3 | `TestSSHExecUnknownCommand` | `internal/sshserver` |
| 4 | `TestHTTPSExecEmptyPath` | `internal/httpserver` |
| 5 | `TestHTTPSConfigGetMethod` | `internal/httpserver` |
| 6 | `TestHTTPSConfigEmptyBody` | `internal/httpserver` |
| 7 | `TestHTTPSExecURLDecoding` | `internal/httpserver` |
| 8 | `TestProxyBadAuth` | `internal/proxy` |
| 9 | `TestProxyBackendDown` | `internal/proxy` |
| 10 | `TestProxyConfigPost` | `internal/proxy` |
| 11 | `TestSelfSignedConfigValid` | `internal/tlsutil` |
| 12 | `TestSetupIdempotent` | `internal/netem` (root) |
| 13 | `TestTeardownNoOp` | `internal/netem` (root) |
| 14 | `TestNetemCampusVsWanDelay` | `internal/netem` (root) |
| 15 | `TestNewMissingDir` | `internal/device` |
| 16 | `TestNewEmptyDir` | `internal/device` |
| 17 | `TestSSHMultipleSessionsOnOneConn` | `integration_test` |
| 18 | `TestHTTPSKeepAlive` | `integration_test` |
| 19 | `TestHTTPSFreshConnDisableKeepAlive` | `integration_test` |
| 20 | `TestConcurrentSSHSessions` | `integration_test` |
| 21 | `TestConcurrentHTTPSRequests` | `integration_test` |
| 22 | `TestProxyPooledConcurrent` | `integration_test` |
| 23 | `TestRunParallelCountsErrors` | `internal/stats` |
| 24 | `TestSSHServerPortInUse` | `internal/sshserver` |
| 25 | `TestHTTPSServerPortInUse` | `internal/httpserver` |
| 26 | `TestDeviceNoTranscripts` | `internal/device` |
| 27 | `TestDeviceMissingDir` | `internal/device` |
| 28 | `TestProxyBackendDiesMidRequest` | `internal/proxy` |

### P2 — Nice to Have

| # | Test | Package |
|---|---|---|
| 1 | `TestSSHBadChannelType` | `internal/sshserver` |
| 2 | `TestSelfSignedConfigUnique` | `internal/tlsutil` |
| 3 | `TestNewNonTxtFilesIgnored` | `internal/device` |
| 4 | `TestListenerNegativeDelay` | `internal/latency` |
| 5 | `TestSSHServerConnDrop` | `internal/sshserver` |
| 6 | `TestHTTPSNoAuthHeader` | `internal/httpserver` |
| 7 | `TestSSHExecLargePayload` | `internal/sshserver` |
| 8 | `TestNetemConcurrentSetup` | `internal/netem` |

---

## 9. Implementation Notes

### Extracting `summarize`/`percentile` for testability

`cmd/bench/main.go` originally had `summarize()`, `percentile()`, `runParallel()`, and `generateExecPayload()` in `package main`. These were extracted to `internal/stats` as exported functions (`Summarize`, `Percentile`, `RunParallel`, `GenerateExecPayload`). `cmd/bench/main.go` now imports from `internal/stats`.

During extraction, the stddev formula was corrected from population stddev (÷N) to sample stddev (÷(N-1), Bessel's correction), with a guard for n==1 returning 0.

### Proxy test setup

Proxy tests need three servers: an SSH backend, the proxy itself, and optionally a direct HTTPS server for comparison. The test helper should:

1. Start SSH server on `:0`
2. Start proxy pointing at the SSH server's address
3. Return both addresses + cleanup

### Time-sensitive tests

Tests that measure actual elapsed time (latency validation) should use generous tolerances (±50%) to avoid flaky CI. The goal is to verify delay is present, not to measure it precisely. Example:

```go
if elapsed < delay/2 {
    t.Errorf("too fast: %v, expected >= %v", elapsed, delay/2)
}
```
