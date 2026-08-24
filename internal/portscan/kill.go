package portscan

import (
	"syscall"
	"time"
)

// EscalateWindow is how long after a SIGTERM a second keypress means SIGKILL.
const EscalateWindow = 2 * time.Second

// Terminate sends SIGTERM to a process.
func Terminate(pid int32) error { return signal(pid, syscall.SIGTERM) }

// Kill sends SIGKILL to a process.
func Kill(pid int32) error { return signal(pid, syscall.SIGKILL) }

func signal(pid int32, sig syscall.Signal) error {
	return syscall.Kill(int(pid), sig)
}

// ShouldEscalate reports whether killing the target identified by key should
// escalate, i.e. the same target was already asked to stop within
// EscalateWindow. Keys come from Listener.Key so that both host processes and
// containers can be tracked.
func ShouldEscalate(lastKey, key string, lastAt, now time.Time) bool {
	if lastKey == "" || lastKey != key || lastAt.IsZero() {
		return false
	}
	return now.Sub(lastAt) <= EscalateWindow
}
