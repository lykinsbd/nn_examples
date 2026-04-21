# SSH vs HTTPS CLI Transport Benchmark

Companion code for the [network-notes.com](https://network-notes.com) blog series on SSH vs HTTPS as a CLI transport for network device management.

## What This Does

A dual-protocol network device emulator and benchmark client that measures the performance difference between SSH and HTTPS for executing CLI commands against network equipment.

The server emulates a Cisco IOS-XE device over both:
- **SSH** (port 2222) — `crypto/ssh` with exec mode, following [CiSSHGo](https://github.com/tbotnz/cisshgo) patterns
- **HTTPS** (port 8443) — TLS + ASA-style HTTP interface (`/admin/exec/`, `/admin/config`)

Both transports share the same command engine and transcript responses, so the only variable is the transport protocol itself.

## Latency Profiles

Simulated latency is injected at the TCP connection level. Profiles are sourced from published backbone measurements:

| Profile | RTT | Represents | Source |
|---|---|---|---|
| `local` | 0ms | Co-located / loopback | Baseline |
| `campus` | 2ms | Same data center / campus | AWS intra-AZ, Prisma 2024 report |
| `regional` | 30ms | Intra-country backbone | Verizon Enterprise Mar 2026: US 29.9ms, EU 15.2ms |
| `continental` | 70ms | Transatlantic (NYC↔London) | Verizon Enterprise Mar 2026: 70.2ms |
| `intercontinental` | 150ms | US↔East Asia | Verizon Enterprise Mar 2026: HK-US 145.5ms |
| `transpacific` | 175ms | US↔Australia/NZ | Verizon Enterprise Mar 2026: NZ 174.2ms |

Source: [Verizon Enterprise Monthly IP Latency Statistics](https://www.verizon.com/business/terms/latency/)

## Quick Start

```bash
go build -o bin/bench ./cmd/bench/

# Baseline (no latency)
./bin/bench -latency local -iterations 50 -commands 3

# Simulated US backbone (30ms RTT)
./bin/bench -latency regional -iterations 20 -commands 3

# Simulated US↔Hong Kong (150ms RTT)
./bin/bench -latency intercontinental -iterations 10 -commands 3
```

## Benchmark Modes

The client tests these scenarios:

| Mode | Description |
|---|---|
| `ssh/fresh-conn` | New TCP + SSH handshake + auth per iteration, exec mode for each command |
| `https/fresh-conn` | New TCP + TLS handshake per command (DisableKeepAlives) |
| `https/keep-alive` | Single TCP + TLS connection reused across commands |
| `https/multi-cmd` | All commands in one HTTP request (ASA `/admin/exec/cmd1/cmd2/cmd3` syntax) |

## Project Structure

```
cmd/
  server/     # Standalone dual-protocol server
  bench/      # Benchmark client (embeds its own server)
  smoketest/  # Integration test
internal/
  device/     # Shared command engine, prefix matching, transcript loading
  sshserver/  # crypto/ssh server (CiSSHGo patterns)
  httpserver/ # net/http + TLS server (ASA-style API)
  latency/    # TCP connection delay injection
transcripts/  # Command output files
```

## License

MIT
