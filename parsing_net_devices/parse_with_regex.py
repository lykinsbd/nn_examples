#!/usr/bin/env python3

"""
Example code for Network-Notes.com entry on Parsing Network Device Output
"""

import getpass
import importlib.resources
import os
import re
import sys


from netmiko import ConnectHandler


"""
Test parsing network device configuration
"""

# Gather the needed credentials
net_device_username = input("Username: ")
net_device_password = getpass.getpass()
# Set enable/secret = password for now
net_device_secret = net_device_password

# Setup a dict with our ASAvs in it, in real world this could be read
# from a CSV or the CLI or any other source
firewalls = {
    "ASAv1": {"ip": "10.10.10.50", "platform": "cisco_asa"},
    "ASAv2": {"ip": "10.10.60.2", "platform": "cisco_asa"},
}

# Setup an empty dict for our results:
results = {}

# Instantiate netmiko connection objects and gather the output of
# `show version | inc So|Serial| up` on these two firewalls
for fw_name, fw_data in firewalls.items():
    print(f"Connecting to {fw_name}...")
    fw_connection = ConnectHandler(
        ip=fw_data["ip"],
        device_type=fw_data["platform"],
        username=net_device_username,
        password=net_device_password,
        secret=net_device_secret,
    )
    results[fw_name] = fw_connection.send_command(
        command_string="show version | inc So|Serial| up"
    )

# Parse our results:
print("Parsing Results...")
parsed_results = {}
for fw_name, result in results.items():
    parsed_results[fw_name] = {}
    parsed_results[fw_name]["version"] = re.search(
        r"Software Version (\d.\d{1,2}(?:\(?\d{1,2}?\)?\d{1,2}?)?)\W*$",
        result,
        re.MULTILINE,
    )
    parsed_results[fw_name]["uptime"] = re.search(
        r"up (.*)$", result, re.MULTILINE
    )
    parsed_results[fw_name]["serial"] = re.search(
        r"Serial Number: (\S*)$", result, re.MULTILINE
    )

# Print our results
for fw_name in results.keys():
    print(f"{fw_name} information:\n")
    print(f"\tVersion: {parsed_results[fw_name]['version'].group(1)}")
    print(f"\tUptime: {parsed_results[fw_name]['uptime'].group(1)}")
    print(f"\tSerial Number: {parsed_results[fw_name]['serial'].group(1)}")

sys.exit()
