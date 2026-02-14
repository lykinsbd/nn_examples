#!/usr/bin/env python3
"""
ISAKMP Test Listener

A simple ISAKMP responder for testing the isakmp_tester.py script.
Listens on UDP/500 and responds to ISAKMP Phase 1 Main Mode packets.

This is NOT a real VPN implementation - it only responds to initial
ISAKMP packets for testing purposes.

Usage:
    sudo python3 isakmp_listener.py [--port 500] [--interface 0.0.0.0]

Requirements:
    - Python 3.8+
    - Scapy
    - Root/sudo access

Author: Brett Lykins
License: Apache 2.0
"""

import argparse
import sys
from scapy.all import *

# Suppress Scapy warnings
import logging
logging.getLogger("scapy.runtime").setLevel(logging.ERROR)


def build_response(request):
    """
    Build a simple ISAKMP response to the request.
    
    Args:
        request: Scapy packet containing ISAKMP request
    
    Returns:
        Response packet or None
    """
    if not request.haslayer(ISAKMP):
        return None
    
    # Extract request details
    req_isakmp = request[ISAKMP]
    
    # Only respond to Main Mode (exch_type=2) or Aggressive Mode (exch_type=4)
    if req_isakmp.exch_type not in [2, 4]:
        print(f"[!] Ignoring non-Phase1 exchange type: {req_isakmp.exch_type}")
        return None
    
    print(f"\n[*] Received ISAKMP packet from {request[IP].src}:{request[UDP].sport}")
    print(f"    Initiator Cookie: {req_isakmp.init_cookie.hex()}")
    print(f"    Exchange Type: {req_isakmp.exch_type} ({'Main Mode' if req_isakmp.exch_type == 2 else 'Aggressive Mode'})")
    
    # Check if SA payload is present
    if not request.haslayer(ISAKMP_payload_SA):
        print(f"[!] No SA payload found")
        return None
    
    # Extract transform details if available
    if request.haslayer(ISAKMP_payload_Transform):
        transform = request[ISAKMP_payload_Transform]
        print(f"    Transform ID: {transform.transform_id}")
        
        if hasattr(transform, 'attributes'):
            print(f"    Attributes:")
            for attr in transform.attributes:
                attr_type = attr.attribute_type
                attr_val = attr.attribute_value
                
                # Decode common attributes
                if attr_type in [0x8001, 0x0001]:
                    print(f"      Encryption: {attr_val}")
                elif attr_type in [0x800e, 0x000e]:
                    print(f"      Key Length: {attr_val}")
                elif attr_type in [0x8002, 0x0002]:
                    print(f"      Hash: {attr_val}")
                elif attr_type in [0x8004, 0x0004]:
                    print(f"      DH Group: {attr_val}")
                elif attr_type in [0x8003, 0x0003]:
                    print(f"      Auth Method: {attr_val}")
    
    # Build response packet
    ip = IP(src=request[IP].dst, dst=request[IP].src)
    udp = UDP(sport=500, dport=request[UDP].sport)
    
    # ISAKMP header with responder cookie
    isakmp = ISAKMP(
        init_cookie=req_isakmp.init_cookie,  # Echo initiator cookie
        resp_cookie=RandString(8),            # Generate responder cookie
        next_payload=1,                       # SA payload
        exch_type=req_isakmp.exch_type,      # Echo exchange type
        flags=0
    )
    
    # Echo back the SA payload (simplified - just accept what was proposed)
    if request.haslayer(ISAKMP_payload_SA):
        sa = request[ISAKMP_payload_SA].copy()
        sa.next_payload = 0
    else:
        # Fallback: create minimal SA payload
        sa = ISAKMP_payload_SA(
            next_payload=0,
            DOI=1,
            situation=1
        )
    
    # Assemble response
    response = ip / udp / isakmp / sa
    
    print(f"[+] Sending response with Responder Cookie: {isakmp.resp_cookie.hex()}")
    
    return response


def start_listener(interface="0.0.0.0", port=500):
    """
    Start ISAKMP listener.
    
    Args:
        interface: Interface to bind to
        port: UDP port to listen on (default: 500)
    """
    print(f"[*] Starting ISAKMP test listener")
    print(f"    Interface: {interface}")
    print(f"    Port: UDP/{port}")
    print(f"[*] Waiting for ISAKMP packets... (Ctrl+C to stop)\n")
    
    def packet_handler(pkt):
        """Handle incoming packets."""
        if pkt.haslayer(ISAKMP):
            response = build_response(pkt)
            if response:
                send(response, verbose=0)
    
    # Sniff for ISAKMP packets
    try:
        sniff(
            filter=f"udp and port {port}",
            prn=packet_handler,
            store=0
        )
    except KeyboardInterrupt:
        print("\n\n[*] Listener stopped by user")


def main():
    parser = argparse.ArgumentParser(
        description='ISAKMP test listener for testing isakmp_tester.py',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s
  %(prog)s --port 500 --interface 0.0.0.0
  
Note: This is a test tool only. It does NOT implement a real VPN.
        """
    )
    
    parser.add_argument('--interface', default='0.0.0.0',
                        help='Interface to bind to (default: 0.0.0.0)')
    parser.add_argument('--port', type=int, default=500,
                        help='UDP port to listen on (default: 500)')
    
    args = parser.parse_args()
    
    # Check for root
    if os.geteuid() != 0:
        print("[!] Error: This script requires root/sudo for raw socket access")
        print("    Run with: sudo python3 isakmp_listener.py")
        sys.exit(1)
    
    try:
        start_listener(args.interface, args.port)
    except Exception as e:
        print(f"\n[!] Error: {e}")
        sys.exit(1)


if __name__ == '__main__':
    main()
