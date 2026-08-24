package portscan

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// FormatUptime renders a duration compactly, e.g. "42s", "5m12s", "3h05m", "2d4h".
func FormatUptime(d time.Duration) string {
	if d <= 0 {
		return "-"
	}

	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

// CommandLabel turns a raw command line into something readable in a narrow
// column: absolute paths are reduced to their base name.
func CommandLabel(name string, cmdline []string) string {
	if len(cmdline) == 0 {
		return name
	}

	parts := make([]string, 0, len(cmdline))
	for _, arg := range cmdline {
		if strings.HasPrefix(arg, "/") {
			arg = filepath.Base(arg)
		}
		if arg == "" {
			continue
		}
		parts = append(parts, arg)
	}
	if len(parts) == 0 {
		return name
	}
	return strings.Join(parts, " ")
}

// Truncate shortens s to width runes, marking the cut with an ellipsis.
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
