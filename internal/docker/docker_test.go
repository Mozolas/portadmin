package docker

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const containersJSON = `[
 {"Id":"6fc2df4a3d50","Names":["/zero-waste-optimizer"],"Image":"omni-waste-optimizer","State":"running",
  "Labels":{"com.docker.compose.project":"omni-waste","com.docker.compose.service":"optimizer"},
  "Ports":[{"IP":"127.0.0.1","PrivatePort":50056,"PublicPort":50056,"Type":"tcp"}]},
 {"Id":"467c97f44523","Names":["/zero-waste-postgres"],"Image":"timescale/timescaledb-ha:pg16","State":"running",
  "Labels":{"com.docker.compose.project":"zero-waste"},
  "Ports":[{"PrivatePort":8008,"Type":"tcp"},
           {"IP":"0.0.0.0","PrivatePort":5432,"PublicPort":5432,"Type":"tcp"},
           {"IP":"::","PrivatePort":5432,"PublicPort":5432,"Type":"tcp"}]},
 {"Id":"nolisten","Names":["/worker"],"Image":"worker:latest","State":"running","Ports":[{"PrivatePort":9000,"Type":"tcp"}]},
 {"Id":"stopped","Names":["/old"],"Image":"old:latest","State":"exited",
  "Ports":[{"PrivatePort":80,"PublicPort":8081,"Type":"tcp"}]},
 {"Id":"udponly","Names":["/dns"],"Image":"dns:latest","State":"running",
  "Ports":[{"PrivatePort":53,"PublicPort":5353,"Type":"udp"}]}
]`

func TestParseContainers(t *testing.T) {
	got, err := parseContainers([]byte(containersJSON))
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("parsed %d containers, want 2 (running, with a published TCP port)", len(got))
	}

	first := got[0]
	if first.Name != "zero-waste-optimizer" || first.ComposeProject != "omni-waste" || first.ComposeService != "optimizer" {
		t.Fatalf("first container = %+v", first)
	}
	if len(first.Ports) != 1 || first.Ports[0] != 50056 {
		t.Fatalf("first container ports = %v, want [50056]", first.Ports)
	}

	// The IPv4 and IPv6 publication of 5432 must collapse into one port.
	if ports := got[1].Ports; len(ports) != 1 || ports[0] != 5432 {
		t.Fatalf("second container ports = %v, want [5432]", ports)
	}
}

func TestTrimImageDigest(t *testing.T) {
	tests := map[string]string{
		"timescale/timescaledb-ha:pg16@sha256:0bb4a5e5c0e2": "timescale/timescaledb-ha:pg16",
		"redis:7":     "redis:7",
		"@sha256:abc": "@sha256:abc",
	}

	for in, want := range tests {
		if got := trimImageDigest(in); got != want {
			t.Fatalf("trimImageDigest(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLabelPrefersComposeProject(t *testing.T) {
	if got := (Container{Name: "api", ComposeProject: "shop"}).Label(); got != "shop" {
		t.Fatalf("Label() = %q, want %q", got, "shop")
	}
	if got := (Container{Name: "api"}).Label(); got != "api" {
		t.Fatalf("Label() = %q, want %q", got, "api")
	}
}

func TestParseStartedAt(t *testing.T) {
	body := []byte(`{"State":{"StartedAt":"2026-08-24T08:42:40.782249346Z","Status":"running"}}`)

	got := parseStartedAt(body)

	want := time.Date(2026, 8, 24, 8, 42, 40, 782249346, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parseStartedAt() = %v, want %v", got, want)
	}
}

func TestParseStartedAtHandlesNeverStartedAndGarbage(t *testing.T) {
	if got := parseStartedAt([]byte(`{"State":{"StartedAt":"0001-01-01T00:00:00Z"}}`)); !got.IsZero() {
		t.Fatalf("never started container returned %v, want zero time", got)
	}
	if got := parseStartedAt([]byte(`not json`)); !got.IsZero() {
		t.Fatalf("garbage returned %v, want zero time", got)
	}
}

func TestSocketPathPrefersDockerHost(t *testing.T) {
	env := map[string]string{"DOCKER_HOST": "unix:///custom/docker.sock", "HOME": "/home/dev"}
	getenv := func(k string) string { return env[k] }

	got := SocketPath(getenv, func(p string) bool { return p == "/custom/docker.sock" })
	if got != "/custom/docker.sock" {
		t.Fatalf("SocketPath() = %q, want the DOCKER_HOST socket", got)
	}
}

func TestSocketPathFallsBackToKnownLocations(t *testing.T) {
	env := map[string]string{"HOME": "/home/dev"}
	getenv := func(k string) string { return env[k] }
	orbstack := filepath.Join("/home/dev", ".orbstack/run/docker.sock")

	got := SocketPath(getenv, func(p string) bool { return p == orbstack })
	if got != orbstack {
		t.Fatalf("SocketPath() = %q, want %q", got, orbstack)
	}
}

func TestSocketPathWithoutDockerReturnsEmpty(t *testing.T) {
	getenv := func(string) string { return "" }

	if got := SocketPath(getenv, func(string) bool { return false }); got != "" {
		t.Fatalf("SocketPath() = %q, want empty", got)
	}
}

// fakeEngine serves the handful of endpoints the client uses over a unix socket.
func fakeEngine(t *testing.T) (*Client, *[]string) {
	t.Helper()

	// Not t.TempDir(): its path contains the test name and a unix socket path
	// is limited to about 100 characters.
	dir, err := os.MkdirTemp("", "pa")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	socket := filepath.Join(dir, "d.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}

	var requests []string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())

		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Write([]byte(containersJSON))
		case strings.HasSuffix(r.URL.Path, "/json"):
			w.Write([]byte(`{"State":{"StartedAt":"2026-08-24T08:42:40Z"}}`))
		case strings.HasSuffix(r.URL.Path, "/stop"), strings.HasSuffix(r.URL.Path, "/kill"):
			if strings.Contains(r.URL.Path, "/containers/missing/") {
				http.Error(w, `{"message":"No such container"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "{\"message\":\"No such container\"}", http.StatusNotFound)
		}
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })

	return newClientForSocket(socket), &requests
}

func TestClientContainersOverUnixSocket(t *testing.T) {
	client, requests := fakeEngine(t)

	containers, err := client.Containers(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(containers) != 2 {
		t.Fatalf("got %d containers, want 2", len(containers))
	}
	want := time.Date(2026, 8, 24, 8, 42, 40, 0, time.UTC)
	if !containers[0].StartedAt.Equal(want) {
		t.Fatalf("StartedAt = %v, want %v", containers[0].StartedAt, want)
	}
	if !strings.Contains((*requests)[0], "/containers/json") {
		t.Fatalf("first request = %q", (*requests)[0])
	}
}

func TestClientCachesStartTimes(t *testing.T) {
	client, requests := fakeEngine(t)

	if _, err := client.Containers(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := len(*requests)
	if _, err := client.Containers(context.Background()); err != nil {
		t.Fatal(err)
	}

	// The second listing must only re-list, not re-inspect every container.
	if added := len(*requests) - after; added != 1 {
		t.Fatalf("second listing made %d requests, want 1 (start times cached)", added)
	}
}

func TestClientStopAndKill(t *testing.T) {
	client, requests := fakeEngine(t)

	if err := client.Stop(context.Background(), "abc123"); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := client.Kill(context.Background(), "abc123"); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}

	joined := strings.Join(*requests, " | ")
	if !strings.Contains(joined, "POST /v1.41/containers/abc123/stop?t=3") {
		t.Fatalf("requests = %s, want a stop with a grace period", joined)
	}
	if !strings.Contains(joined, "POST /v1.41/containers/abc123/kill") {
		t.Fatalf("requests = %s, want a kill", joined)
	}
}

func TestClientReportsEngineErrors(t *testing.T) {
	client, _ := fakeEngine(t)

	err := client.Stop(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "No such container") {
		t.Fatalf("Stop() error = %v, want the engine message", err)
	}
}

func TestNilClientIsUsable(t *testing.T) {
	var client *Client

	containers, err := client.Containers(context.Background())
	if err != nil || containers != nil {
		t.Fatalf("Containers() = %v, %v; want nil, nil", containers, err)
	}
	if err := client.Stop(context.Background(), "abc"); err == nil {
		t.Fatal("Stop() on a nil client should report that docker is unavailable")
	}
}
