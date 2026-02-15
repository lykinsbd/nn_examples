#!/usr/bin/env python3
"""
ISAKMP Test Listener

A simple ISAKMP responder for testing the isakmp_tester.py script.
Listens on UDP/500 and responds to ISAKMP Phase 1 Main Mode packets.

This is NOT a real VPN implementation - it only responds to initial
ISAKMP packets for testing purposes.

Usage:
    sudo poetry run python isakmp_listener.py [--port 500] [--interface 0.0.0.0]

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
    try:
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
            
            # Parse transforms (list of tuples)
            if hasattr(transform, 'transforms') and transform.transforms:
                print(f"    Transform Attributes:")
                for attr_type, attr_val in transform.transforms:
                    # Decode common attributes
                    if attr_type == 1:
                        print(f"      Encryption: {attr_val}")
                    elif attr_type == 14:
                        print(f"      Key Length: {attr_val}")
                    elif attr_type == 2:
                        print(f"      Hash: {attr_val}")
                    elif attr_type == 4:
                        print(f"      DH Group: {attr_val}")
                    elif attr_type == 3:
                        print(f"      Auth Method: {attr_val}")
                    elif attr_type == 11:
                        print(f"      Life Type: {attr_val}")
                    elif attr_type == 12:
                        print(f"      Life Duration: {attr_val}")
        
        # Build response packet
        ip = IP(src=request[IP].dst, dst=request[IP].src)
        udp = UDP(sport=500, dport=request[UDP].sport)
        
        print(f"[*] Building response: {request[IP].dst} -> {request[IP].src}")
        
        # ISAKMP header with responder cookie
        resp_cookie_val = RandString(8)._fix()  # Generate and fix the random value
        isakmp = ISAKMP(
            init_cookie=req_isakmp.init_cookie,  # Echo initiator cookie
            resp_cookie=resp_cookie_val,          # Generate responder cookie
            exch_type=req_isakmp.exch_type,      # Echo exchange type
        )
        
        # Echo back the SA payload (accept what was proposed)
        # Build a simple accepting SA payload
        sa = ISAKMP_payload_SA(
            doi=1,
            situation=1
        )
        
        # Try to copy the proposal if it exists
        if request.haslayer(ISAKMP_payload_SA) and request.haslayer(ISAKMP_payload_Proposal):
            try:
                sa.prop = request[ISAKMP_payload_Proposal]
            except:
                pass  # Use minimal SA if copy fails
        
        # Assemble response
        response = ip / udp / isakmp / sa
        
        print(f"[+] Response built, Responder Cookie: {resp_cookie_val.hex() if isinstance(resp_cookie_val, bytes) else resp_cookie_val}")
        
        return response
    
    except Exception as e:
        print(f"[!] Error building response: {e}")
        import traceback
        traceback.print_exc()
        return None


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
    
    # Track seen cookies to avoid loops
    seen_cookies = set()
    
    def packet_handler(pkt):
        """Handle incoming packets."""
        if pkt.haslayer(ISAKMP):
            # Check if this is a request (responder cookie is all zeros)
            req_isakmp = pkt[ISAKMP]
            if req_isakmp.resp_cookie != b'\x00' * 8:
                # This is a response, not a request - ignore it
                return
            
            # Check if we've already seen this initiator cookie
            init_cookie = req_isakmp.init_cookie
            if init_cookie in seen_cookies:
                return
            
            seen_cookies.add(init_cookie)
            
            response = build_response(pkt)
            if response:
                print(f"[*] Sending response...")
                # Use sendp for layer 2 to ensure proper delivery on same machine
                # Add Ether layer if not present
                if not response.haslayer(Ether) and pkt.haslayer(Ether):
                    # Copy Ether layer from request, swap src/dst
                    ether = Ether(src=pkt[Ether].dst, dst=pkt[Ether].src)
                    response = ether / response
                    sendp(response, verbose=0)
                    print(f"[+] Response sent via layer 2")
                else:
                    # Fallback to layer 3 send
                    send(response, verbose=0)
                    print(f"[+] Response sent via layer 3")
            else:
                print(f"[!] No response built")
    
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
        print("    Run with: sudo poetry run python isakmp_listener.py")
        sys.exit(1)
    
    try:
        start_listener(args.interface, args.port)
    except Exception as e:
        print(f"\n[!] Error: {e}")
        sys.exit(1)


if __name__ == '__main__':
    main()
