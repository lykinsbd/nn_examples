// Package netem provides tc netem-based latency injection on Linux.
// Requires CAP_NET_ADMIN (or root). Applies per-port delay on the
// loopback interface using a prio qdisc with u32 filters.
//
// Qdiscs are managed via vishvananda/netlink. Filters use exec("tc")
// because the netlink u32 filter API interacts poorly with prio's
// priomap — filters added via netlink don't override the priomap
// classification in all kernel versions.
package netem

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/vishvananda/netlink"
)

const loopbackIndex = 1

// Setup configures tc netem on the loopback interface with per-port delays.
func Setup(wanDelay, campusDelay time.Duration, wanPorts, campusPorts []int) error {
	lo, err := netlink.LinkByIndex(loopbackIndex)
	if err != nil {
		return fmt.Errorf("get loopback: %w", err)
	}
	_ = lo

	// Clean any existing root qdisc
	Teardown()

	// Root prio qdisc: 4 bands, all-zero priomap (unmatched → band 0, no delay)
	prio := netlink.NewPrio(netlink.QdiscAttrs{
		LinkIndex: loopbackIndex,
		Handle:    netlink.MakeHandle(1, 0),
		Parent:    netlink.HANDLE_ROOT,
	})
	prio.Bands = 4
	prio.PriorityMap = [16]uint8{}
	if err := netlink.QdiscAdd(prio); err != nil {
		return fmt.Errorf("add prio qdisc: %w", err)
	}

	// Band 2: WAN delay
	if wanDelay > 0 {
		netem := netlink.NewNetem(
			netlink.QdiscAttrs{LinkIndex: loopbackIndex, Handle: netlink.MakeHandle(20, 0), Parent: netlink.MakeHandle(1, 2)},
			netlink.NetemQdiscAttrs{Latency: uint32(wanDelay.Microseconds())},
		)
		if err := netlink.QdiscAdd(netem); err != nil {
			return fmt.Errorf("add wan netem: %w", err)
		}
		for _, port := range wanPorts {
			if err := addFilter(port, "1:2"); err != nil {
				return err
			}
		}
	}

	// Band 3: campus delay
	if campusDelay > 0 {
		netem := netlink.NewNetem(
			netlink.QdiscAttrs{LinkIndex: loopbackIndex, Handle: netlink.MakeHandle(30, 0), Parent: netlink.MakeHandle(1, 3)},
			netlink.NetemQdiscAttrs{Latency: uint32(campusDelay.Microseconds())},
		)
		if err := netlink.QdiscAdd(netem); err != nil {
			return fmt.Errorf("add campus netem: %w", err)
		}
		for _, port := range campusPorts {
			if err := addFilter(port, "1:3"); err != nil {
				return err
			}
		}
	}

	return nil
}

// addFilter uses tc(8) to add u32 port filters. The netlink u32 API
// doesn't reliably override prio priomap classification, so we shell
// out for this one operation.
func addFilter(port int, flowid string) error {
	p := fmt.Sprintf("%d", port)
	for _, args := range [][]string{
		{"filter", "add", "dev", "lo", "parent", "1:0", "protocol", "ip", "u32", "match", "ip", "dport", p, "0xffff", "flowid", flowid},
		{"filter", "add", "dev", "lo", "parent", "1:0", "protocol", "ip", "u32", "match", "ip", "sport", p, "0xffff", "flowid", flowid},
	} {
		if out, err := exec.Command("tc", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("tc %v: %s (%w)", args[:5], out, err)
		}
	}
	return nil
}

// Teardown removes the tc qdisc from loopback.
func Teardown() {
	netlink.QdiscDel(netlink.NewPrio(netlink.QdiscAttrs{
		LinkIndex: loopbackIndex,
		Handle:    netlink.MakeHandle(1, 0),
		Parent:    netlink.HANDLE_ROOT,
	}))
}
