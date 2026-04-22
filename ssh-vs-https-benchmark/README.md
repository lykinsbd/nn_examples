# SSH vs HTTPS CLI Transport Benchmark

Companion code for the [CLI Over HTTPS](https://network-notes.com/posts/2026/cli-over-https-1/)
blog series on network-notes.com. Measures the performance difference between
SSH and HTTPS as CLI transports for network device automation at scale.

## What This Does

A dual-protocol network device emulator and benchmark client. The server
emulates a Cisco IOS-XE device over both:

- **SSH** (port 2222) — `crypto/ssh` with exec mode, following
  [CiSSHGo](https://github.com/tbotnz/cisshgo) patterns
- **HTTPS** (port 8443) — TLS 1.3 + ASA-style HTTP interface
  (`/admin/exec/`, `/admin/config`)
- **Proxy** (port 9443) — HTTPS frontend that forwards to an SSH backend,
  simulating the edge proxy pattern

Both transports share the same command engine and transcript responses, so
the only variable is the transport protocol itself.

## Quick Start

```bash
go build -o bin/bench ./cmd/bench/

# Baseline (no latency)
./bin/bench -latency local -iterations 50 -commands 5

# Simulated US backbone (30ms RTT)
./bin/bench -latency regional -iterations 20 -commands 5

# Simulated US↔Hong Kong (150ms RTT)
./bin/bench -latency intercontinental -iterations 20 -commands 5

# Proxy mode only
./bin/bench -latency regional -iterations 20 -commands 5 -transport proxy
```

Output is JSON to stdout. Logs go to stderr.

## Benchmark Modes

| Mode | Transport | Description |
|---|---|---|
| `fresh-conn` | SSH | New TCP + SSH handshake + auth per iteration |
| `reuse-conn` | SSH | Shared connection, new channel per command (ControlMaster-style) |
| `batch-exec` | SSH | All commands in one exec payload |
| `fresh-conn` | HTTPS | New TCP + TLS handshake per iteration |
| `keep-alive` | HTTPS | Shared TLS connection across all iterations |
| `batch-post` | HTTPS | All commands in one POST body |
| `multi-cmd` | HTTPS | All commands in one GET (ASA `/cmd1/cmd2` syntax) |
| `fresh-ssh` | Proxy | HTTPS→proxy→fresh SSH per request |
| `pooled-ssh` | Proxy | HTTPS→proxy→pooled SSH connection |

## Latency Profiles

| Profile | RTT | Represents | Source |
|---|---|---|---|
| `local` | 0ms | Co-located / loopback | Baseline |
| `campus` | 2ms | Same data center / campus | AWS intra-AZ |
| `regional` | 30ms | Intra-country backbone | Verizon Mar 2026: US 29.9ms |
| `continental` | 70ms | Transatlantic | Verizon Mar 2026: 70.2ms |
| `intercontinental` | 150ms | US↔East Asia | Verizon Mar 2026: HK-US 145.5ms |
| `transpacific` | 175ms | US↔Australia/NZ | Verizon Mar 2026: NZ 174.2ms |

Source: [Verizon Enterprise Monthly IP Latency Statistics](https://www.verizon.com/business/terms/latency/)

## Methodology and Caveats

### How latency injection works

Latency is injected at the `net.Conn` level using a wrapper that adds a
configurable one-way delay. The delay fires on **direction changes** — when
a connection switches from reading to writing or vice versa — not on every
individual `Read()` or `Write()` syscall. This models the fact that
consecutive writes in the same direction are typically coalesced into a
single TCP flight, while a direction change (e.g., sending a request then
waiting for a response) incurs a network round trip.

### What this does NOT model

- **TCP congestion, packet loss, or jitter.** All connections are
  localhost with deterministic delay. Real networks have variance.
- **Kernel-level TCP behavior.** Tools like `tc netem` inject delay at the
  kernel level and capture effects like Nagle's algorithm, delayed ACKs,
  and TCP window scaling. Our userspace wrapper cannot replicate these.
- **Real device processing time.** The emulated device responds instantly.
  Real devices have CPU overhead for parsing, AAA lookups, and command
  execution that adds to total latency.
- **TLS session resumption.** The HTTPS fresh-conn benchmark does a full
  TLS 1.3 handshake every time. Real clients may use session tickets to
  reduce subsequent handshakes to 1-RTT or 0-RTT.

### Why the results are still directionally valid

The core finding — that HTTPS requires fewer round trips than SSH for the
same CLI operation — is a property of the protocol design, not the latency
model. SSH's channel-open/exec/data/close sequence requires more
request-response exchanges than HTTPS's single request-response. This
structural difference holds regardless of how latency is injected.

At zero latency (local profile), SSH actually beats HTTPS because the TLS
handshake has higher CPU overhead than the SSH handshake. The HTTPS
advantage only appears when network latency dominates, which is the
scenario the blog series focuses on.

### Reproducing with tc netem

For kernel-level validation on Linux:

```bash
# Add 15ms one-way delay to loopback (30ms RTT)
sudo tc qdisc add dev lo root netem delay 15ms

# Run benchmark without the built-in latency injection
./bin/bench -latency local -iterations 20 -commands 5

# Clean up
sudo tc qdisc del dev lo root
```

## Running Tests

```bash
go test -race ./...
```

## Sample Results

The `results/` directory contains sample JSON output from benchmark runs.
These are generated with small iteration counts for illustration; the blog
series uses n=20 for all published numbers.

## Project Structure

```
cmd/
  bench/      # Benchmark client (embeds its own server)
  server/     # Standalone dual-protocol server
  smoketest/  # Quick integration smoke test
internal/
  device/     # Command engine, prefix matching, transcript loading
  sshserver/  # crypto/ssh server
  httpserver/ # net/http + TLS server (ASA-style API)
  latency/    # TCP connection delay injection (direction-change model)
  proxy/      # HTTPS→SSH edge proxy (fresh + pooled modes)
  tlsutil/    # Shared self-signed TLS config generator
transcripts/  # Canned command output files
results/      # Sample benchmark output (JSON)
```

## License

MIT
