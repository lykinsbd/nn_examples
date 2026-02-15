# ISAKMP Testing with Scapy

Python scripts for testing ISAKMP/IKE Phase 1 connectivity using Scapy.

## Requirements

- Python 3.8+
- Poetry (for dependency management)
- Root/sudo access (required for raw sockets)

## Installation

### Install Poetry (if not already installed)

```bash
curl -sSL https://install.python-poetry.org | python3 -
```

### Install Dependencies

```bash
cd isakmp_testing
poetry install

# Also install for root (required for raw socket access)
sudo poetry install
```

> **Note:** Scapy requires raw socket access, which needs root privileges. Since `sudo` runs commands in a separate environment, you must install dependencies both as your user and as root.

## Usage

### Test Single Transform Set

```bash
sudo poetry run python isakmp_tester.py 192.168.1.1
```

### Test Multiple Transform Sets

```bash
sudo poetry run python isakmp_tester.py 192.168.1.1 --test-multiple
```

### Use Aggressive Mode

```bash
sudo poetry run python isakmp_tester.py 192.168.1.1 --aggressive
```

### Custom Transform Set

```bash
sudo poetry run python isakmp_tester.py 192.168.1.1 \
    --encryption 7 \
    --key-length 256 \
    --hash 4 \
    --dh-group 14 \
    --timeout 10
```

## Test Listener

A test listener is included for development and debugging:

```bash
# Start the listener
sudo poetry run python isakmp_listener.py
```

The listener accepts all proposed transform sets and logs packet details.

**Intended Use:** The listener is designed for testing between **separate machines** or for debugging packet structure. It receives ISAKMP packets and logs their contents, which is useful for:
- Verifying packet structure
- Testing between two different hosts
- Debugging transform set encoding
- Learning ISAKMP packet formats

**Same-Machine Limitation:** Testing the tester and listener on the same machine has limitations due to how the kernel handles local routing and how Scapy sends responses. The listener will receive and log packets, but responses may not reach the tester. This is expected behavior and not a bug.

For reliable testing, use the listener on a **separate machine** from the tester, or test against a real ISAKMP/VPN device.

## Options

- `--test-multiple`: Test multiple common transform sets
- `--aggressive`: Use Aggressive Mode instead of Main Mode
- `--timeout N`: Response timeout in seconds (default: 5)
- `--encryption N`: Encryption algorithm (7=AES-CBC, 5=3DES-CBC)
- `--key-length N`: Key length in bits (128, 192, 256)
- `--hash N`: Hash algorithm (4=SHA-256, 2=SHA-1)
- `--dh-group N`: DH group (14=2048-bit, 5=1536-bit, 2=1024-bit)
- `--lifetime N`: SA lifetime in seconds (default: 86400)

## Security Considerations

⚠️ **Authorization Required**: Only test systems you own or have explicit permission to test.

🔒 **Cryptographic Security**: Use modern parameters:
- Hash: SHA-256 or SHA-384 (avoid SHA-1)
- Encryption: AES-256-CBC or AES-256-GCM
- DH Group: 14+ (2048-bit or higher)

## Troubleshooting

### Raw Socket Access

Scapy requires raw socket access, which needs root/sudo privileges:

```bash
# This won't work (no raw socket access)
poetry run python isakmp_tester.py 192.168.1.1

# This works
sudo poetry run python isakmp_tester.py 192.168.1.1
```

**Important:** Since `sudo` runs in a separate environment, you must install dependencies as root:

```bash
sudo poetry install
```

This creates a separate Poetry virtual environment for root with the required dependencies.

## Related Blog Posts

- [Testing ISAKMP: Basic Connectivity - Part 1](https://network-notes.com/posts/2016/netcat-isakmp/)
- [Testing ISAKMP: Pre-Built Packets - Part 2](https://network-notes.com/posts/2026/netcat-isakmp-2/)
- [Testing ISAKMP: Building Packets from Scratch - Part 3](https://network-notes.com/posts/2026/netcat-isakmp-3/)
- [Testing ISAKMP: Using Scapy - Part 4](https://network-notes.com/posts/2026/netcat-isakmp-4/)

## License

Apache 2.0
