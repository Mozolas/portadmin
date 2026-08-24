package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Mozolas/portadmin/internal/portscan"
)

func testModel(t *testing.T) (model, *[]string) {
	t.Helper()

	var calls []string
	m := newModel()
	m.terminate = func(pid int32) error {
		calls = append(calls, "TERM")
		return nil
	}
	m.kill = func(pid int32) error {
		calls = append(calls, "KILL")
		return nil
	}
	m.now = func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) }

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return next.(model), &calls
}

func sampleListeners() []portscan.Listener {
	return []portscan.Listener{
		{PID: 111, Port: 3000, Project: "storefront", Command: "next dev", Uptime: 90 * time.Second},
		{PID: 222, Port: 5432, Project: "postgres-db", Command: "postgres -D data", Uptime: 3 * time.Hour},
	}
}

func TestViewRendersColumnsAndRows(t *testing.T) {
	m, _ := testModel(t)
	m = m.applyScan(listenersMsg{listeners: sampleListeners()})

	view := m.View()
	for _, want := range []string{"PORT", "PROJECT", "COMMAND", "PID", "UPTIME", "3000", "storefront", "next dev", "111", "1m30s", "5432", "3h00m"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestApplyScanKeepsCursorOnTheSameProcess(t *testing.T) {
	m, _ := testModel(t)
	m = m.applyScan(listenersMsg{listeners: sampleListeners()})
	m.table.SetCursor(1)

	// The first process disappears; the cursor must follow PID 222.
	m = m.applyScan(listenersMsg{listeners: sampleListeners()[1:]})

	selected, ok := m.selected()
	if !ok || selected.PID != 222 {
		t.Fatalf("selected = %+v (ok=%v), want PID 222", selected, ok)
	}
}

func TestApplyScanReportsScanError(t *testing.T) {
	m, _ := testModel(t)
	m = m.applyScan(listenersMsg{err: errScan{}})

	if m.statusKind != statusError || !strings.Contains(m.status, "permission denied") {
		t.Fatalf("status = %q (kind %v), want a scan error", m.status, m.statusKind)
	}
}

type errScan struct{}

func (errScan) Error() string { return "permission denied" }

func TestEnterSendsSIGTERMThenSIGKILL(t *testing.T) {
	m, calls := testModel(t)
	m = m.applyScan(listenersMsg{listeners: sampleListeners()})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if len(*calls) != 1 || (*calls)[0] != "TERM" {
		t.Fatalf("calls = %v, want one SIGTERM", *calls)
	}
	if !strings.Contains(m.status, "SIGTERM") {
		t.Fatalf("status = %q, want a SIGTERM message", m.status)
	}

	// Second press within the escalation window.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if len(*calls) != 2 || (*calls)[1] != "KILL" {
		t.Fatalf("calls = %v, want SIGTERM then SIGKILL", *calls)
	}
	if !strings.Contains(m.status, "SIGKILL") {
		t.Fatalf("status = %q, want a SIGKILL message", m.status)
	}
}

func TestSecondPressAfterWindowSendsSIGTERMAgain(t *testing.T) {
	m, calls := testModel(t)
	m = m.applyScan(listenersMsg{listeners: sampleListeners()})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	base := m.now()
	m.now = func() time.Time { return base.Add(3 * time.Second) }

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if len(*calls) != 2 || (*calls)[1] != "TERM" {
		t.Fatalf("calls = %v, want two SIGTERMs", *calls)
	}
}

func TestKillTargetsTheSelectedRow(t *testing.T) {
	m, _ := testModel(t)
	m = m.applyScan(listenersMsg{listeners: sampleListeners()})

	var killed int32
	m.terminate = func(pid int32) error {
		killed = pid
		return nil
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(model)

	if killed != 222 {
		t.Fatalf("killed PID %d, want 222 (second row after moving down with j)", killed)
	}
}

func TestQuitKey(t *testing.T) {
	m, _ := testModel(t)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("cmd produced %T, want tea.QuitMsg", msg)
	}
}
