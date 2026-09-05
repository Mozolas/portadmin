package portscan

import (
	"context"
	"time"

	"github.com/Mozolas/portadmin/internal/docker"
)

// ContainerSource lists the running containers that publish a port. It is an
// interface so the scan can be tested without a Docker engine.
type ContainerSource interface {
	Containers(ctx context.Context) ([]docker.Container, error)
}

// ScanWithContainers combines the host processes with the containers that
// published the ports, so a published port shows the container instead of the
// runtime's proxy process.
func ScanWithContainers(ctx context.Context, src ContainerSource) ([]Listener, error) {
	return scanWith(ctx, Scan, src)
}

func scanWith(ctx context.Context, scan func() ([]Listener, error), src ContainerSource) ([]Listener, error) {
	listeners, err := scan()
	if err != nil {
		return nil, err
	}
	if src == nil {
		return listeners, nil
	}

	containers, cerr := src.Containers(ctx)
	if cerr != nil {
		// Docker being unavailable is not a reason to show nothing.
		return listeners, nil
	}
	return mergeContainers(listeners, containers, time.Now()), nil
}

// mergeContainers rewrites every listener whose port is published by a
// container and appends the published ports that have no host process.
func mergeContainers(listeners []Listener, containers []docker.Container, now time.Time) []Listener {
	byPort := map[uint32]docker.Container{}
	for _, c := range containers {
		for _, port := range c.Ports {
			byPort[port] = c
		}
	}

	matched := map[uint32]bool{}
	merged := make([]Listener, 0, len(listeners)+len(byPort))
	for _, l := range listeners {
		if c, ok := byPort[l.Port]; ok {
			matched[l.Port] = true
			merged = append(merged, containerListener(c, l.Port, l.PID, now))
			continue
		}
		merged = append(merged, l)
	}

	for port, c := range byPort {
		if !matched[port] {
			merged = append(merged, containerListener(c, port, 0, now))
		}
	}

	sortListeners(merged)
	return merged
}

func containerListener(c docker.Container, port uint32, pid int32, now time.Time) Listener {
	l := Listener{
		PID:           pid,
		Port:          port,
		Project:       c.Label(),
		Command:       containerCommand(c),
		ContainerID:   c.ID,
		ContainerName: c.Name,
	}
	if !c.StartedAt.IsZero() {
		l.Started = c.StartedAt
		l.Uptime = now.Sub(c.StartedAt)
	}
	return l
}

func containerCommand(c docker.Container) string {
	name := c.Name
	if name == "" {
		name = shortID(c.ID)
	}
	if c.Image == "" {
		return "docker: " + name
	}
	return "docker: " + name + " · " + c.Image
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
