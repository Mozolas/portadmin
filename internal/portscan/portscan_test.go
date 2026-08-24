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
