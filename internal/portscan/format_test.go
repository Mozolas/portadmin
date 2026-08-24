package portscan

import (
	"testing"
	"time"
)

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"zero", 0, "-"},
		{"negative", -time.Second, "-"},
		{"seconds", 42 * time.Second, "42s"},
		{"minutes", 5*time.Minute + 12*time.Second, "5m12s"},
		{"minutes pad seconds", 5*time.Minute + 3*time.Second, "5m03s"},
		{"hours", 3*time.Hour + 5*time.Minute, "3h05m"},
		{"days", 50 * time.Hour, "2d2h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatUptime(tt.in); got != tt.want {
				t.Fatalf("FormatUptime(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCommandLabelShortensAbsolutePaths(t *testing.T) {
	got := CommandLabel("node", []string{"/usr/local/bin/node", "/Users/dev/app/server.js", "--port", "3000"})

	want := "node server.js --port 3000"
	if got != want {
		t.Fatalf("CommandLabel() = %q, want %q", got, want)
	}
}

func TestCommandLabelFallsBackToProcessName(t *testing.T) {
	if got := CommandLabel("postgres", nil); got != "postgres" {
		t.Fatalf("CommandLabel() = %q, want %q", got, "postgres")
	}
	if got := CommandLabel("postgres", []string{""}); got != "postgres" {
		t.Fatalf("CommandLabel() = %q, want %q", got, "postgres")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in    string
		width int
		want  string
	}{
		{"vite", 10, "vite"},
		{"next dev --turbo", 8, "next de…"},
		{"abc", 1, "…"},
		{"abc", 0, ""},
		{"přílišžluťoučký", 6, "příli…"},
	}

	for _, tt := range tests {
		if got := Truncate(tt.in, tt.width); got != tt.want {
			t.Fatalf("Truncate(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
		}
	}
}
