package docker

import (
	"encoding/json"
	"io"
	"strings"
	"time"
)

type apiContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Labels map[string]string `json:"Labels"`
	Ports  []struct {
		PrivatePort uint32 `json:"PrivatePort"`
		PublicPort  uint32 `json:"PublicPort"`
		Type        string `json:"Type"`
	} `json:"Ports"`
}

// parseContainers keeps running containers that publish a TCP port on the host.
func parseContainers(body []byte) ([]Container, error) {
	var raw []apiContainer
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	containers := make([]Container, 0, len(raw))
	for _, r := range raw {
		if r.State != "" && r.State != "running" {
			continue
		}

		seen := map[uint32]bool{}
		var ports []uint32
		for _, p := range r.Ports {
			// A container bound to both 0.0.0.0 and :: reports the port twice.
			if p.PublicPort == 0 || (p.Type != "" && p.Type != "tcp") || seen[p.PublicPort] {
				continue
			}
			seen[p.PublicPort] = true
			ports = append(ports, p.PublicPort)
		}
		if len(ports) == 0 {
			continue
		}

		containers = append(containers, Container{
			ID:             r.ID,
			Name:           containerName(r.Names),
			Image:          trimImageDigest(r.Image),
			ComposeProject: r.Labels["com.docker.compose.project"],
			ComposeService: r.Labels["com.docker.compose.service"],
			Ports:          ports,
		})
	}
	return containers, nil
}

// trimImageDigest drops the "@sha256:…" suffix the engine reports for images
// pulled by digest; the tag alone is what identifies the image to a human.
func trimImageDigest(image string) string {
	if i := strings.Index(image, "@sha256:"); i > 0 {
		return image[:i]
	}
	return image
}

func containerName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

func parseStartedAt(body []byte) time.Time {
	var inspect struct {
		State struct {
			StartedAt string `json:"StartedAt"`
		} `json:"State"`
	}
	if err := json.Unmarshal(body, &inspect); err != nil {
		return time.Time{}
	}

	at, err := time.Parse(time.RFC3339Nano, inspect.State.StartedAt)
	if err != nil {
		return time.Time{}
	}
	// A container that never started reports the zero timestamp.
	if at.Year() < 2000 {
		return time.Time{}
	}
	return at
}

func readAll(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
