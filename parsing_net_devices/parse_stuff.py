#!/usr/bin/env python3

"""
Example code for Network-Notes.com blog entry on Parsing Network Device Output
"""

import getpass
import importlib.resources
import os
import re
import sys


from netmiko import ConnectHandler


def main():
    """
    Test parsing network device configuration
    """

    # First, we need to set the NET_TEXTFSM environment variable, so that netmiko knows where our templates are
    with importlib.resources.path(package="ntc_templates", resource="templates") as template_path:
        os.environ["NET_TEXTFSM"] = str(template_path)

    # Next, gather the needed credentials
    net_device_username = input("Username: ")
    net_device_password = getpass.getpass()
    net_device_secret = net_device_password  # We're just setting the enable/secret to the same as the password for now

    # Setup a dict with our ASAvs in it, in real world this could be read from a CSV or the CLI or any other source
    firewalls = {
        "ASAv1": {"ip": "10.10.10.50", "platform": "cisco_asa"},
        "ASAv2": {"ip": "10.10.60.2", "platform": "cisco_asa"},
    }

    # Setup an empty dict for our results:
    results = {}

    # Instantiate netmiko connection objects and gather the output of `show version` on these two firewalls
    for fw_name, fw_data in firewalls.items():
        print(f"Connecting to {fw_name}...")
        fw_connection = ConnectHandler(
            ip=fw_data["ip"],
            device_type=fw_data["platform"],
            username=net_device_username,
            password=net_device_password,
            secret=net_device_secret,
        )
        results[fw_name] = fw_connection.send_command(command_string="show version")

    # Print our results
    for fw_name, result in results.items():
        print(f"{fw_name} version information:\n")
        print(result)

    sys.exit()


if __name__ == "__main__":
    main()
