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

// ShouldEscalate reports whether killing pid now should use SIGKILL, i.e. the
// same process was already sent a SIGTERM within EscalateWindow.
func ShouldEscalate(lastPID int32, lastAt time.Time, pid int32, now time.Time) bool {
	if lastPID != pid || lastAt.IsZero() {
		return false
	}
	return now.Sub(lastAt) <= EscalateWindow
}
