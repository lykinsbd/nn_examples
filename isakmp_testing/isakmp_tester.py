#!/usr/bin/env python3
"""
ISAKMP/IKE Phase 1 Tester using Scapy

This script tests ISAKMP/IKE Phase 1 connectivity and transform set acceptance
using Scapy to build and send packets dynamically.

Usage:
    python isakmp_tester.py <target_ip> [options]

Examples:
    # Test single target with default transform set
    python isakmp_tester.py 192.168.1.1
    
    # Test with multiple transform sets
    python isakmp_tester.py 192.168.1.1 --test-multiple
    
    # Use Aggressive Mode instead of Main Mode
    python isakmp_tester.py 192.168.1.1 --aggressive

Requirements:
    - Python 3.8+
    - Scapy (pip install scapy)
    - Root/sudo access for raw sockets

Author: Brett Lykins
License: Apache 2.0
"""

import argparse
import sys
import time
from scapy.all import *

# Suppress Scapy warnings
import logging
logging.getLogger("scapy.runtime").setLevel(logging.ERROR)


# Transform set constants
ENCR_AES_CBC = 7
ENCR_3DES_CBC = 5
HASH_SHA256 = 4
HASH_SHA1 = 2
DH_GROUP_14 = 14
DH_GROUP_5 = 5
DH_GROUP_2 = 2
AUTH_PSK = 1


def build_isakmp_packet(target_ip, transform_set, aggressive_mode=False):
    """
    Build a complete ISAKMP Phase 1 packet.
    
    Args:
        target_ip: Target IP address
        transform_set: Dictionary with encryption, hash, dh_group, auth_method, lifetime
        aggressive_mode: Use Aggressive Mode (True) or Main Mode (False)
    
    Returns:
        Complete Scapy packet
    """
    # IP and UDP layers
    ip = IP(dst=target_ip)
    udp = UDP(sport=500, dport=500)
    
    # ISAKMP header
    isakmp = ISAKMP(
        init_cookie=RandString(8),
        resp_cookie=b'\x00' * 8,
        next_payload=1,
        exch_type=4 if aggressive_mode else 2,  # 4=Aggressive, 2=Main Mode
        flags=0
    )
    
    # Build transform attributes
    trans_attrs = []
    
    # Encryption algorithm (TV format)
    trans_attrs.append(ISAKMP_payload_Transform_Attribute(
        attribute_type=0x8001,
        attribute_value=transform_set['encryption']
    ))
    
    # Key length (TV format) - only if needed
    if transform_set.get('key_length', 0) > 0:
        trans_attrs.append(ISAKMP_payload_Transform_Attribute(
            attribute_type=0x800e,
            attribute_value=transform_set['key_length']
        ))
    
    # Hash algorithm (TV format)
    trans_attrs.append(ISAKMP_payload_Transform_Attribute(
        attribute_type=0x8002,
        attribute_value=transform_set['hash']
    ))
    
    # DH Group (TV format)
    trans_attrs.append(ISAKMP_payload_Transform_Attribute(
        attribute_type=0x8004,
        attribute_value=transform_set['dh_group']
    ))
    
    # Authentication method (TV format)
    trans_attrs.append(ISAKMP_payload_Transform_Attribute(
        attribute_type=0x8003,
        attribute_value=transform_set['auth_method']
    ))
    
    # Life type (TV format)
    trans_attrs.append(ISAKMP_payload_Transform_Attribute(
        attribute_type=0x800b,
        attribute_value=1  # Seconds
    ))
    
    # Life duration (TLV format)
    trans_attrs.append(ISAKMP_payload_Transform_Attribute(
        attribute_type=0x000c,
        length=4,
        attribute_value=transform_set['lifetime']
    ))
    
    # Build Transform payload
    transform = ISAKMP_payload_Transform(
        next_payload=0,
        transform_type=1,
        transform_id=1,
        attributes=trans_attrs
    )
    
    # Build Proposal payload
    proposal = ISAKMP_payload_Proposal(
        next_payload=0,
        proposal=1,
        proto=1,
        SPIsize=0,
        trans_nb=1,
        trans=transform
    )
    
    # Build SA payload
    sa = ISAKMP_payload_SA(
        next_payload=0,
        DOI=1,
        situation=1,
        prop=proposal
    )
    
    # Assemble packet
    packet = ip / udp / isakmp / sa
    
    return packet


def send_and_parse(packet, timeout=5):
    """
    Send ISAKMP packet and parse response.
    
    Args:
        packet: Scapy packet to send
        timeout: Response timeout in seconds
    
    Returns:
        Dictionary with response details or None
    """
    response = sr1(packet, timeout=timeout, verbose=0)
    
    if not response or not response.haslayer(ISAKMP):
        return None
    
    result = {
        'responder_cookie': response[ISAKMP].resp_cookie.hex(),
        'exchange_type': response[ISAKMP].exch_type,
        'accepted': response.haslayer(ISAKMP_payload_SA)
    }
    
    return result


def test_single_transform(target_ip, transform_set, aggressive_mode=False, timeout=5):
    """Test a single transform set against target."""
    print(f"\n[*] Testing {target_ip}")
    print(f"    Mode: {'Aggressive' if aggressive_mode else 'Main Mode'}")
    print(f"    Encryption: {transform_set['encryption']}")
    print(f"    Hash: {transform_set['hash']}")
    print(f"    DH Group: {transform_set['dh_group']}")
    
    packet = build_isakmp_packet(target_ip, transform_set, aggressive_mode)
    result = send_and_parse(packet, timeout)
    
    if result:
        if result['accepted']:
            print(f"[+] ACCEPTED - Transform set accepted by peer")
            print(f"    Responder Cookie: {result['responder_cookie']}")
            return True
        else:
            print(f"[-] REJECTED - No SA payload in response")
            return False
    else:
        print(f"[-] NO RESPONSE - Timeout or no ISAKMP response")
        return False


def test_multiple_transforms(target_ip, timeout=5):
    """Test multiple common transform sets."""
    transform_sets = [
        {
            'name': 'Modern Strong (AES-256/SHA-256/DH14)',
            'encryption': ENCR_AES_CBC,
            'key_length': 256,
            'hash': HASH_SHA256,
            'dh_group': DH_GROUP_14,
            'auth_method': AUTH_PSK,
            'lifetime': 86400
        },
        {
            'name': 'Modern Moderate (AES-128/SHA-256/DH14)',
            'encryption': ENCR_AES_CBC,
            'key_length': 128,
            'hash': HASH_SHA256,
            'dh_group': DH_GROUP_14,
            'auth_method': AUTH_PSK,
            'lifetime': 86400
        },
        {
            'name': 'Legacy Strong (AES-256/SHA-1/DH5)',
            'encryption': ENCR_AES_CBC,
            'key_length': 256,
            'hash': HASH_SHA1,
            'dh_group': DH_GROUP_5,
            'auth_method': AUTH_PSK,
            'lifetime': 86400
        },
        {
            'name': 'Legacy Moderate (3DES/SHA-1/DH5)',
            'encryption': ENCR_3DES_CBC,
            'key_length': 0,
            'hash': HASH_SHA1,
            'dh_group': DH_GROUP_5,
            'auth_method': AUTH_PSK,
            'lifetime': 86400
        },
        {
            'name': 'Legacy Weak (3DES/SHA-1/DH2)',
            'encryption': ENCR_3DES_CBC,
            'key_length': 0,
            'hash': HASH_SHA1,
            'dh_group': DH_GROUP_2,
            'auth_method': AUTH_PSK,
            'lifetime': 86400
        }
    ]
    
    print(f"\n{'='*60}")
    print(f"Testing {len(transform_sets)} transform sets against {target_ip}")
    print(f"{'='*60}")
    
    results = []
    for i, ts in enumerate(transform_sets, 1):
        print(f"\n[{i}/{len(transform_sets)}] {ts['name']}")
        
        packet = build_isakmp_packet(target_ip, ts)
        result = send_and_parse(packet, timeout)
        
        accepted = result and result['accepted']
        results.append({'name': ts['name'], 'accepted': accepted})
        
        if accepted:
            print(f"    [+] ACCEPTED")
        else:
            print(f"    [-] REJECTED or no response")
        
        # Small delay between tests
        if i < len(transform_sets):
            time.sleep(1)
    
    # Summary
    print(f"\n{'='*60}")
    print("SUMMARY")
    print(f"{'='*60}")
    accepted_count = sum(1 for r in results if r['accepted'])
    print(f"Accepted: {accepted_count}/{len(results)} transform sets\n")
    
    if accepted_count > 0:
        print("Accepted transform sets:")
        for r in results:
            if r['accepted']:
                print(f"  [+] {r['name']}")
    
    return results


def main():
    parser = argparse.ArgumentParser(
        description='ISAKMP/IKE Phase 1 tester using Scapy',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s 192.168.1.1
  %(prog)s 192.168.1.1 --test-multiple
  %(prog)s 192.168.1.1 --aggressive --timeout 10
        """
    )
    
    parser.add_argument('target', help='Target IP address')
    parser.add_argument('--test-multiple', action='store_true',
                        help='Test multiple common transform sets')
    parser.add_argument('--aggressive', action='store_true',
                        help='Use Aggressive Mode instead of Main Mode')
    parser.add_argument('--timeout', type=int, default=5,
                        help='Response timeout in seconds (default: 5)')
    parser.add_argument('--encryption', type=int, default=ENCR_AES_CBC,
                        help=f'Encryption algorithm (default: {ENCR_AES_CBC} for AES-CBC)')
    parser.add_argument('--key-length', type=int, default=256,
                        help='Key length in bits (default: 256)')
    parser.add_argument('--hash', type=int, default=HASH_SHA256,
                        help=f'Hash algorithm (default: {HASH_SHA256} for SHA-256)')
    parser.add_argument('--dh-group', type=int, default=DH_GROUP_14,
                        help=f'DH group (default: {DH_GROUP_14} for 2048-bit)')
    parser.add_argument('--lifetime', type=int, default=86400,
                        help='SA lifetime in seconds (default: 86400)')
    
    args = parser.parse_args()
    
    # Check for root
    if os.geteuid() != 0:
        print("[!] Warning: This script requires root/sudo for raw socket access")
        print("    Run with: sudo python3 isakmp_tester.py ...")
        sys.exit(1)
    
    try:
        if args.test_multiple:
            test_multiple_transforms(args.target, args.timeout)
        else:
            transform_set = {
                'encryption': args.encryption,
                'key_length': args.key_length,
                'hash': args.hash,
                'dh_group': args.dh_group,
                'auth_method': AUTH_PSK,
                'lifetime': args.lifetime
            }
            test_single_transform(args.target, transform_set, 
                                args.aggressive, args.timeout)
    
    except KeyboardInterrupt:
        print("\n\n[!] Interrupted by user")
        sys.exit(0)
    except Exception as e:
        print(f"\n[!] Error: {e}")
        sys.exit(1)


if __name__ == '__main__':
    main()
