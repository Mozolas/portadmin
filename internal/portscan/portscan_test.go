package portscan

import (
	"syscall"
	"testing"

	gnet "github.com/shirou/gopsutil/v4/net"
)

func conn(pid int32, port uint32, status string, sockType uint32) gnet.ConnectionStat {
	return gnet.ConnectionStat{
		Pid:    pid,
		Type:   sockType,
		Status: status,
		Laddr:  gnet.Addr{IP: "127.0.0.1", Port: port},
	}
}

func TestListeningEndpointsKeepsOnlyListeningTCP(t *testing.T) {
	conns := []gnet.ConnectionStat{
		conn(101, 3000, "LISTEN", syscall.SOCK_STREAM),
		conn(102, 5432, "ESTABLISHED", syscall.SOCK_STREAM),
		conn(103, 5353, "NONE", syscall.SOCK_DGRAM),
	}

	got := listeningEndpoints(conns)

	want := []Endpoint{{PID: 101, Port: 3000}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("listeningEndpoints() = %v, want %v", got, want)
	}
}

func TestListeningEndpointsDeduplicatesIPv4AndIPv6(t *testing.T) {
	conns := []gnet.ConnectionStat{
		conn(200, 8080, "LISTEN", syscall.SOCK_STREAM),
		conn(200, 8080, "LISTEN", syscall.SOCK_STREAM),
	}

	if got := listeningEndpoints(conns); len(got) != 1 {
		t.Fatalf("expected a single endpoint, got %v", got)
	}
}

func TestListeningEndpointsSkipsUnknownPIDsAndPorts(t *testing.T) {
	conns := []gnet.ConnectionStat{
		conn(0, 3000, "LISTEN", syscall.SOCK_STREAM),
		conn(-1, 3001, "LISTEN", syscall.SOCK_STREAM),
		conn(300, 0, "LISTEN", syscall.SOCK_STREAM),
	}

	if got := listeningEndpoints(conns); len(got) != 0 {
		t.Fatalf("expected no endpoints, got %v", got)
	}
}

func TestListeningEndpointsSortedByPortThenPID(t *testing.T) {
	conns := []gnet.ConnectionStat{
		conn(10, 9000, "LISTEN", syscall.SOCK_STREAM),
		conn(30, 3000, "LISTEN", syscall.SOCK_STREAM),
		conn(20, 3000, "LISTEN", syscall.SOCK_STREAM),
	}

	got := listeningEndpoints(conns)

	want := []Endpoint{{PID: 20, Port: 3000}, {PID: 30, Port: 3000}, {PID: 10, Port: 9000}}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("listeningEndpoints() = %v, want %v", got, want)
		}
	}
}

func ports(listeners []Listener) []uint32 {
	out := make([]uint32, len(listeners))
	for i, l := range listeners {
		out[i] = l.Port
	}
	return out
}

func TestSortListenersGroupsAProjectTogether(t *testing.T) {
	listeners := []Listener{
		{PID: 1, Port: 3000, Project: "storefront"},
		{PID: 2, Port: 4000, Project: "@acme/api"},
		{PID: 3, Port: 5173, Project: "storefront"},
		{PID: 4, Port: 5432, Project: "@acme/api"},
	}

	sortListeners(listeners)

	// Both storefront ports first, because 3000 is the lowest of any group.
	want := []uint32{3000, 5173, 4000, 5432}
	if got := ports(listeners); !equalPorts(got, want) {
		t.Fatalf("ports = %v, want %v", got, want)
	}
}

func TestSortListenersLeavesNamelessRowsInPortOrder(t *testing.T) {
	listeners := []Listener{
		{PID: 1, Port: 8080},
		{PID: 2, Port: 3000, Project: "storefront"},
		{PID: 3, Port: 22},
		{PID: 4, Port: 9000, Project: "storefront"},
	}

	sortListeners(listeners)

	// The two unnamed daemons keep their own places; they are not a project.
	want := []uint32{22, 3000, 9000, 8080}
	if got := ports(listeners); !equalPorts(got, want) {
		t.Fatalf("ports = %v, want %v", got, want)
	}
}

func equalPorts(got, want []uint32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestParseLsofCwds(t *testing.T) {
	out := "p123\nfcwd\nn/Users/dev/api\np456\nfcwd\nn/Users/dev/web\n"

	got := parseLsofCwds(out)

	if got[123] != "/Users/dev/api" || got[456] != "/Users/dev/web" {
		t.Fatalf("parseLsofCwds() = %v", got)
	}
}

func TestParseLsofCwdsIgnoresGarbage(t *testing.T) {
	out := "nope\npNaN\nn/Users/dev/api\n\np789\nn/tmp\n"

	got := parseLsofCwds(out)

	if len(got) != 1 || got[789] != "/tmp" {
		t.Fatalf("parseLsofCwds() = %v, want only pid 789", got)
	}
}
