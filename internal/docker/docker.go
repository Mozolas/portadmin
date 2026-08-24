// Package docker talks to the local Docker Engine API over its unix socket, so
// a published port can be shown as the container that actually owns it instead
// of the proxy process running on the host.
package docker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// apiVersion is old enough to be served by every current engine (Docker
// Desktop, OrbStack, Colima, Rancher Desktop, plain dockerd).
const apiVersion = "v1.41"

const (
	requestTimeout = 2 * time.Second
	stopTimeout    = 3 // seconds the engine gives the container to shut down
	startedAtTTL   = 30 * time.Second
)

// Container is a running container with at least one published port.
type Container struct {
	ID             string
	Name           string
	Image          string
	ComposeProject string
	ComposeService string
	Ports          []uint32
	StartedAt      time.Time
}

// Label returns the name to show for the container.
func (c Container) Label() string {
	if c.ComposeProject != "" {
		return c.ComposeProject
	}
	return c.Name
}

// Client is a minimal Docker Engine API client. A nil client is valid and
// simply reports no containers, which is what happens when Docker is not
// running.
type Client struct {
	http   *http.Client
	socket string

	mu      sync.Mutex
	started map[string]startedEntry
}

type startedEntry struct {
	at      time.Time
	fetched time.Time
}

// New returns a client for the local engine, or nil if no socket was found.
func New() *Client {
	socket := SocketPath(os.Getenv, fileExists)
	if socket == "" {
		return nil
	}
	return newClientForSocket(socket)
}

func newClientForSocket(socket string) *Client {
	return &Client{
		socket: socket,
		http: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socket)
				},
			},
		},
		started: map[string]startedEntry{},
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}

// SocketPath picks the engine socket: DOCKER_HOST wins, otherwise the well
// known locations of the common Docker runtimes are probed in order.
func SocketPath(getenv func(string) string, exists func(string) bool) string {
	if host := getenv("DOCKER_HOST"); strings.HasPrefix(host, "unix://") {
		path := strings.TrimPrefix(host, "unix://")
		if exists(path) {
			return path
		}
		return ""
	}

	home := getenv("HOME")
	candidates := []string{
		"/var/run/docker.sock",
		filepath.Join(home, ".docker/run/docker.sock"),
		filepath.Join(home, ".orbstack/run/docker.sock"),
		filepath.Join(home, ".colima/default/docker.sock"),
		filepath.Join(home, ".rd/docker.sock"),
		filepath.Join(home, ".lima/default/sock/docker.sock"),
	}
	for _, path := range candidates {
		if home == "" && strings.Contains(path, "/.") {
			continue
		}
		if exists(path) {
			return path
		}
	}
	return ""
}

// Containers lists running containers that publish at least one TCP port.
func (c *Client) Containers(ctx context.Context) ([]Container, error) {
	if c == nil {
		return nil, nil
	}

	body, err := c.do(ctx, http.MethodGet, "/containers/json")
	if err != nil {
		return nil, err
	}

	containers, err := parseContainers(body)
	if err != nil {
		return nil, err
	}

	for i := range containers {
		containers[i].StartedAt = c.startedAt(ctx, containers[i].ID)
	}
	return containers, nil
}

// Stop asks the engine to stop a container gracefully (SIGTERM, then SIGKILL
// after a short grace period).
func (c *Client) Stop(ctx context.Context, id string) error {
	if c == nil {
		return fmt.Errorf("docker is not available")
	}
	_, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/containers/%s/stop?t=%d", id, stopTimeout))
	return err
}

// Kill sends SIGKILL to a container's main process.
func (c *Client) Kill(ctx context.Context, id string) error {
	if c == nil {
		return fmt.Errorf("docker is not available")
	}
	_, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/containers/%s/kill", id))
	return err
}

func (c *Client) do(ctx context.Context, method, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://docker/"+apiVersion+path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readAll(resp.Body, 4<<20)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("docker api: %s: %s", resp.Status, strings.TrimSpace(firstLine(string(body))))
	}
	return body, nil
}

// startedAt caches container start times, which only change on a restart.
func (c *Client) startedAt(ctx context.Context, id string) time.Time {
	c.mu.Lock()
	entry, ok := c.started[id]
	c.mu.Unlock()
	if ok && time.Since(entry.fetched) < startedAtTTL {
		return entry.at
	}

	body, err := c.do(ctx, http.MethodGet, "/containers/"+id+"/json")
	if err != nil {
		return entry.at
	}
	at := parseStartedAt(body)

	c.mu.Lock()
	c.started[id] = startedEntry{at: at, fetched: time.Now()}
	c.mu.Unlock()
	return at
}
