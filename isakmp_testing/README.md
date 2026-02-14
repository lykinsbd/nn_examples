# ISAKMP Testing with Scapy

Python scripts for testing ISAKMP/IKE Phase 1 connectivity using Scapy.

## Requirements

- Python 3.8+
- Scapy
- Root/sudo access (required for raw sockets)

## Installation

```bash
pip install -r requirements.txt
```

## Usage

### Test Single Transform Set

```bash
sudo python3 isakmp_tester.py 192.168.1.1
```

### Test Multiple Transform Sets

```bash
sudo python3 isakmp_tester.py 192.168.1.1 --test-multiple
```

### Use Aggressive Mode

```bash
sudo python3 isakmp_tester.py 192.168.1.1 --aggressive
```

### Custom Transform Set

```bash
sudo python3 isakmp_tester.py 192.168.1.1 \
    --encryption 7 \
    --key-length 256 \
    --hash 4 \
    --dh-group 14 \
    --timeout 10
```

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

## Related Blog Posts

- [Testing ISAKMP: Basic Connectivity - Part 1](https://network-notes.com/posts/2016/netcat-isakmp/)
- [Testing ISAKMP: Pre-Built Packets - Part 2](https://network-notes.com/posts/2026/netcat-isakmp-2/)
- [Testing ISAKMP: Building Packets from Scratch - Part 3](https://network-notes.com/posts/2026/netcat-isakmp-3/)
- [Testing ISAKMP: Using Scapy - Part 4](https://network-notes.com/posts/2026/netcat-isakmp-4/)

## License

Apache 2.0
