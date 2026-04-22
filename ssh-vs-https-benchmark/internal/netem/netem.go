// Package netem provides tc netem-based latency injection on Linux.
// Requires root or CAP_NET_ADMIN. Applies per-port delay on the
// loopback interface using a prio qdisc with u32 filters.
package netem

import (
	"fmt"
	"os/exec"
	"time"
)

// Setup configures tc netem on the loopback interface with per-port delays.
// wanPorts get wanDelay; campusPorts get campusDelay; all other traffic is unaffected.
func Setup(wanDelay, campusDelay time.Duration, wanPorts, campusPorts []int) error {
	// Clean any existing qdisc
	exec.Command("sudo", "tc", "qdisc", "del", "dev", "lo", "root").Run()

	// Root prio qdisc with 4 bands; default band 1 (no delay)
	if err := run("tc", "qdisc", "add", "dev", "lo", "root", "handle", "1:",
		"prio", "bands", "4", "priomap", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0"); err != nil {
		return fmt.Errorf("prio qdisc: %w", err)
	}

	// Band 2: WAN delay
	if wanDelay > 0 {
		ms := fmt.Sprintf("%dms", wanDelay.Milliseconds())
		if err := run("tc", "qdisc", "add", "dev", "lo", "parent", "1:2", "handle", "20:", "netem", "delay", ms); err != nil {
			return fmt.Errorf("wan netem: %w", err)
		}
		for _, port := range wanPorts {
			p := fmt.Sprintf("%d", port)
			mask := "0xffff"
			if err := run("tc", "filter", "add", "dev", "lo", "parent", "1:0", "protocol", "ip", "u32",
				"match", "ip", "dport", p, mask, "flowid", "1:2"); err != nil {
				return fmt.Errorf("filter dport %d: %w", port, err)
			}
			if err := run("tc", "filter", "add", "dev", "lo", "parent", "1:0", "protocol", "ip", "u32",
				"match", "ip", "sport", p, mask, "flowid", "1:2"); err != nil {
				return fmt.Errorf("filter sport %d: %w", port, err)
			}
		}
	}

	// Band 3: campus delay (for proxy backend)
	if campusDelay > 0 {
		ms := fmt.Sprintf("%dms", campusDelay.Milliseconds())
		if err := run("tc", "qdisc", "add", "dev", "lo", "parent", "1:3", "handle", "30:", "netem", "delay", ms); err != nil {
			return fmt.Errorf("campus netem: %w", err)
		}
		for _, port := range campusPorts {
			p := fmt.Sprintf("%d", port)
			mask := "0xffff"
			if err := run("tc", "filter", "add", "dev", "lo", "parent", "1:0", "protocol", "ip", "u32",
				"match", "ip", "dport", p, mask, "flowid", "1:3"); err != nil {
				return fmt.Errorf("filter dport %d: %w", port, err)
			}
			if err := run("tc", "filter", "add", "dev", "lo", "parent", "1:0", "protocol", "ip", "u32",
				"match", "ip", "sport", p, mask, "flowid", "1:3"); err != nil {
				return fmt.Errorf("filter sport %d: %w", port, err)
			}
		}
	}

	return nil
}

// Teardown removes the tc qdisc from loopback.
func Teardown() error {
	return run("tc", "qdisc", "del", "dev", "lo", "root")
}

func run(args ...string) error {
	cmd := exec.Command("sudo", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, out)
	}
	return nil
}
