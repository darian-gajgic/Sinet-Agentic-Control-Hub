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
// sandboxed process TREE's /proc/<pid>/net/tcp{,6} LISTEN rows — the netns view,
// no nsenter and no privilege (Spec S13.8: "the platform probes the netns for
// listening ports rather than trusting config"). Config is never the port
// source. The probe walks the process tree because bwrap's MONITOR process lives
// in the HOST netns while the actual sandboxed process (and its listener) is a
// descendant sharing the new empty netns — probing the monitor's pid alone would
// read the host listen table (8481/8482 included). The pure parser is
// fixture-tested; a caps-gated live probe (probe_test.go) proves the netns
// semantics against a real bwrap empty netns by consuming THIS production
// function. When more than one port listens the result is picker DATA for S15
// (R8); a single port auto-selects.

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

// probeListeningPorts returns the distinct listening ports of the sandboxed
// process TREE rooted at pid — the union across pid and all its descendants (the
// sandboxed listener shares its netns with the rest of the bwrap sandbox tree;
// the bwrap monitor at the root lives in the host netns and simply contributes
// no sandbox port). This is the "sandboxed process tree" the R8 probe parses.
// Picker DATA for S15; a single port auto-selects (R8).
func probeListeningPorts(pid int) ([]Port, error) {
	seen := map[int]bool{}
	var nums []int
	for _, p := range append(descendants(pid), pid) {
		ports, err := probePID(p)
		if err != nil {
			// A descendant that exited between the /proc scan and the read is not
			// a probe failure — skip it; a read error on the ROOT propagates.
			if p == pid {
				return nil, err
			}
			continue
		}
		for _, n := range ports {
			if !seen[n] {
				seen[n] = true
				nums = append(nums, n)
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

// probePID reads one process's netns listen table (tcp + tcp6). An absent tcp6
// file is not an error (IPv6-less hosts).
func probePID(pid int) ([]int, error) {
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
	return nums, nil
}

// descendants returns every descendant pid of root by walking /proc PPIDs.
func descendants(root int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	ppid := map[int]int{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if pp := parentPID(pid); pp != 0 {
			ppid[pid] = pp
		}
	}
	var out []int
	frontier := []int{root}
	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]
		for pid, pp := range ppid {
			if pp == cur {
				out = append(out, pid)
				frontier = append(frontier, pid)
			}
		}
	}
	return out
}

// parentPID reads /proc/<pid>/stat, tolerating a comm field with spaces/parens.
func parentPID(pid int) int {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	s := string(b)
	i := strings.LastIndex(s, ")") // end of the (comm) field
	if i < 0 {
		return 0
	}
	fields := strings.Fields(s[i+1:]) // state, ppid, ...
	if len(fields) < 2 {
		return 0
	}
	pp, _ := strconv.Atoi(fields[1])
	return pp
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
