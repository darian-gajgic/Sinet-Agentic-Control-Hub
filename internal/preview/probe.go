package preview

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// probe.go discovers a sandboxed dev server's listening ports by PARSING the
// process's own /proc/<pid>/net/tcp{,6} LISTEN rows — the process's netns view,
// no nsenter and no privilege (Spec S13.8: "the platform probes the netns for
// listening ports rather than trusting config"). Config is never the port
// source. The pure parser is fixture-tested; a caps-gated live probe (probe_
// test.go) proves the netns semantics against a real bwrap empty netns. When
// more than one port listens the result is picker DATA for S15 (R8); a single
// port auto-selects.

// stateListen is TCP_LISTEN in the /proc/net/tcp st column (hex 0x0A).
const stateListen = "0A"

// parseProcNetTCP extracts the distinct LISTEN ports from one /proc/net/tcp or
// /proc/net/tcp6 stream. The local-address column is "hexIP:hexPORT"; the port
// is the hex after the final colon (identical for tcp and tcp6). Rows in any
// non-LISTEN state are ignored — config is never trusted, only the kernel's own
// listen table.
func parseProcNetTCP(r io.Reader) ([]int, error) {
	sc := bufio.NewScanner(r)
	seen := map[int]bool{}
	var ports []int
	header := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if header { // the "sl local_address …" header row
			header = false
			continue
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[3] != stateListen {
			continue
		}
		local := fields[1]
		i := strings.LastIndex(local, ":")
		if i < 0 {
			continue
		}
		p, err := strconv.ParseUint(local[i+1:], 16, 32)
		if err != nil {
			continue
		}
		port := int(p)
		if !seen[port] {
			seen[port] = true
			ports = append(ports, port)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sort.Ints(ports)
	return ports, nil
}

// probeListeningPorts reads a process's netns listen table (tcp + tcp6) and
// returns the distinct listening ports as picker DATA (Spec S13.8; R8). An
// absent tcp6 file is not an error (IPv6-less hosts).
func probeListeningPorts(pid int) ([]Port, error) {
	seen := map[int]bool{}
	var nums []int
	for _, proto := range []string{"tcp", "tcp6"} {
		f, err := os.Open(fmt.Sprintf("/proc/%d/net/%s", pid, proto))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("preview: read /proc/%d/net/%s: %w", pid, proto, err)
		}
		ports, err := parseProcNetTCP(f)
		f.Close()
		if err != nil {
			return nil, err
		}
		for _, p := range ports {
			if !seen[p] {
				seen[p] = true
				nums = append(nums, p)
			}
		}
	}
	sort.Ints(nums)
	out := make([]Port, 0, len(nums))
	for _, n := range nums {
		out = append(out, Port{Number: n})
	}
	return out, nil
}

// selectPort resolves the probe result to a primary backend port and whether a
// multi-port picker is needed (Spec S13.8: single port auto-selects; multi-port
// yields picker DATA, R8). The lowest port is the provisional primary; the S15
// picker lets the user change when needsPicker is true.
func selectPort(ports []Port) (primary int, needsPicker bool, ok bool) {
	switch len(ports) {
	case 0:
		return 0, false, false
	case 1:
		return ports[0].Number, false, true
	default:
		return ports[0].Number, true, true
	}
}
