package portscan

import (
	"bufio"
	"os/exec"
	"testing"
	"time"
)

func TestShouldEscalate(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		lastKey string
		lastAt  time.Time
		key     string
		want    bool
	}{
		{"no previous kill", "", time.Time{}, "pid:4242", false},
		{"same process within window", "pid:4242", now.Add(-1500 * time.Millisecond), "pid:4242", true},
		{"same process at window edge", "pid:4242", now.Add(-2 * time.Second), "pid:4242", true},
		{"same process after window", "pid:4242", now.Add(-2100 * time.Millisecond), "pid:4242", false},
		{"different process", "pid:111", now.Add(-time.Second), "pid:4242", false},
		{"same container within window", "container:abc123", now.Add(-time.Second), "container:abc123", true},
		{"container and process with the same number", "pid:4242", now.Add(-time.Second), "container:4242", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldEscalate(tt.lastKey, tt.key, tt.lastAt, now); got != tt.want {
				t.Fatalf("ShouldEscalate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTerminateStopsARunningProcess(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	done := waitAsync(cmd)

	if err := Terminate(int32(cmd.Process.Pid)); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process survived SIGTERM")
	}
}

func TestKillStopsAProcessIgnoringSIGTERM(t *testing.T) {
	// A shell that ignores SIGTERM only goes away with SIGKILL. The loop keeps
	// the shell alive; without it sh would exec the last command and lose the trap.
	cmd := exec.Command("sh", "-c", "trap '' TERM; echo ready; while :; do sleep 0.2; done")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := int32(cmd.Process.Pid)
	defer cmd.Process.Kill()

	// Wait for the trap to be installed before signalling.
	if _, err := bufio.NewReader(stdout).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	done := waitAsync(cmd)

	if err := Terminate(pid); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	select {
	case <-done:
		t.Fatal("process exited on SIGTERM although it ignores the signal")
	case <-time.After(500 * time.Millisecond):
	}

	if err := Kill(pid); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process survived SIGKILL")
	}
}

func waitAsync(cmd *exec.Cmd) <-chan error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return done
}
