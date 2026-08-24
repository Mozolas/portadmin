package portscan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Mozolas/portadmin/internal/docker"
)

var scanTime = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

func testContainers() []docker.Container {
	return []docker.Container{
		{
			ID:             "467c97f44523",
			Name:           "zero-waste-postgres",
			Image:          "timescale/timescaledb-ha:pg16",
			ComposeProject: "zero-waste",
			ComposeService: "postgres",
			Ports:          []uint32{5432},
			StartedAt:      scanTime.Add(-12 * time.Hour),
		},
		{
			ID:        "aabbccddeeff",
			Name:      "standalone-redis",
			Image:     "redis:7",
			Ports:     []uint32{6379, 16379},
			StartedAt: scanTime.Add(-30 * time.Minute),
		},
	}
}

func TestMergeContainersReplacesTheProxyProcess(t *testing.T) {
	// On macOS every published port is held by the same runtime helper process.
	listeners := []Listener{
		{PID: 99391, Port: 5432, Project: "-", Command: "OrbStack Helper", Uptime: 49 * time.Hour},
		{PID: 53840, Port: 4000, Project: "@acme/api", Command: "node dist/main.js", Uptime: time.Hour},
	}

	merged := mergeContainers(listeners, testContainers(), scanTime)

	byPort := map[uint32]Listener{}
	for _, l := range merged {
		byPort[l.Port] = l
	}

	postgres := byPort[5432]
	if postgres.Project != "zero-waste" {
		t.Fatalf("project = %q, want the compose project", postgres.Project)
	}
	if postgres.Command != "docker: zero-waste-postgres · timescale/timescaledb-ha:pg16" {
		t.Fatalf("command = %q", postgres.Command)
	}
	if postgres.ContainerID != "467c97f44523" || !postgres.IsContainer() {
		t.Fatalf("container id = %q", postgres.ContainerID)
	}
	if postgres.PID != 99391 {
		t.Fatalf("PID = %d, want the host process that holds the port", postgres.PID)
	}
	// The uptime must be the container's, not the helper's.
	if postgres.Uptime != 12*time.Hour {
		t.Fatalf("uptime = %v, want 12h", postgres.Uptime)
	}

	// A plain host process is left untouched.
	if api := byPort[4000]; api.Project != "@acme/api" || api.IsContainer() {
		t.Fatalf("host process was rewritten: %+v", api)
	}
}

func TestMergeContainersAddsPublishedPortsWithoutAHostProcess(t *testing.T) {
	merged := mergeContainers(nil, testContainers(), scanTime)

	if len(merged) != 3 {
		t.Fatalf("got %d rows, want 3 published ports", len(merged))
	}
	for _, l := range merged {
		if !l.IsContainer() || l.PID != 0 {
			t.Fatalf("row %+v should be a container row without a PID", l)
		}
	}
	if merged[0].Port != 5432 || merged[1].Port != 6379 || merged[2].Port != 16379 {
		t.Fatalf("rows are not sorted by port: %v", []uint32{merged[0].Port, merged[1].Port, merged[2].Port})
	}
}

func TestMergeContainersUsesContainerNameWithoutCompose(t *testing.T) {
	merged := mergeContainers(nil, testContainers()[1:], scanTime)

	if merged[0].Project != "standalone-redis" {
		t.Fatalf("project = %q, want the container name", merged[0].Project)
	}
	if merged[0].Uptime != 30*time.Minute {
		t.Fatalf("uptime = %v, want 30m", merged[0].Uptime)
	}
}

func TestMergeContainersWithoutStartTimeHasNoUptime(t *testing.T) {
	containers := []docker.Container{{ID: "x", Name: "no-start", Ports: []uint32{9999}}}

	merged := mergeContainers(nil, containers, scanTime)

	if merged[0].Uptime != 0 || !merged[0].Started.IsZero() {
		t.Fatalf("uptime = %v, want none", merged[0].Uptime)
	}
}

func TestContainerKeysAreDistinctFromPIDs(t *testing.T) {
	container := Listener{PID: 0, ContainerID: "4242"}
	process := Listener{PID: 4242}

	if container.Key() == process.Key() {
		t.Fatalf("container and process share the key %q", container.Key())
	}
}

type stubSource struct {
	containers []docker.Container
	err        error
}

func (s stubSource) Containers(context.Context) ([]docker.Container, error) {
	return s.containers, s.err
}

func TestScanWithContainersSurvivesADockerError(t *testing.T) {
	hostOnly := func() ([]Listener, error) {
		return []Listener{{PID: 53840, Port: 4000, Project: "@acme/api"}}, nil
	}

	// Docker being down must not hide the host processes.
	listeners, err := scanWith(context.Background(), hostOnly, stubSource{err: errors.New("connection refused")})
	if err != nil {
		t.Fatalf("scanWith() error = %v, want the host listeners anyway", err)
	}
	if len(listeners) != 1 || listeners[0].Project != "@acme/api" {
		t.Fatalf("listeners = %+v, want the host process untouched", listeners)
	}
}

func TestScanWithContainersWithoutDockerReturnsHostProcesses(t *testing.T) {
	hostOnly := func() ([]Listener, error) {
		return []Listener{{PID: 1, Port: 80}}, nil
	}

	listeners, err := scanWith(context.Background(), hostOnly, nil)
	if err != nil || len(listeners) != 1 {
		t.Fatalf("scanWith() = %+v, %v", listeners, err)
	}
}

func TestScanWithContainersPropagatesScanErrors(t *testing.T) {
	failing := func() ([]Listener, error) { return nil, errors.New("no permission") }

	if _, err := scanWith(context.Background(), failing, stubSource{}); err == nil {
		t.Fatal("expected the scan error to be reported")
	}
}
