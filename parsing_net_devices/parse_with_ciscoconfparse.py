#!/usr/bin/env python3

"""
Example code for Network-Notes.com entry on Parsing Network Device Output
"""

import getpass
import sys


from ciscoconfparse import CiscoConfParse
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
# `show run interface` on these two firewalls
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
        command_string="show run interface"
    )

# Parse our results:
print("Parsing Results...")
outside_interfaces = {}
for fw_name, config in results.items():
    interface_config = CiscoConfParse(config=config.splitlines())
    outside_interfaces[fw_name] = interface_config.find_objects_w_child(
        parentspec=r"^interface", childspec="security-level 0"
    )

# Print our results
for fw_name, interface_list in outside_interfaces.items():
    print(f"==== ==== [ {fw_name} \"Outside\" interfaces ] ==== ====\n")
    for interface in interface_list:
        print(f"     ==== [ {interface.text} ] ====     \n")
        for line in interface.ioscfg:
            print(f"{line}")
        print("\n")

sys.exit()
