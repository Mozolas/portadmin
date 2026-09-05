package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Mozolas/portadmin/internal/docker"
	"github.com/Mozolas/portadmin/internal/portscan"
)

type fakeDocker struct {
	containers []docker.Container
	calls      []string
	stopErr    error
}

func (f *fakeDocker) Containers(context.Context) ([]docker.Container, error) {
	return f.containers, nil
}

func (f *fakeDocker) Stop(_ context.Context, id string) error {
	f.calls = append(f.calls, "stop:"+id)
	return f.stopErr
}

func (f *fakeDocker) Kill(_ context.Context, id string) error {
	f.calls = append(f.calls, "kill:"+id)
	return nil
}

func testModel(t *testing.T) (model, *[]string) {
	t.Helper()

	var calls []string
	m := newModel(nil)
	m.terminate = func(int32) error {
		calls = append(calls, "TERM")
		return nil
	}
	m.kill = func(int32) error {
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

func containerListener() portscan.Listener {
	return portscan.Listener{
		Port:          8080,
		Project:       "zero-waste",
		Command:       "docker: zero-waste-keycloak · quay.io/keycloak/keycloak:26.0",
		ContainerID:   "49a5e1d8e0e0",
		ContainerName: "zero-waste-keycloak",
		Uptime:        12 * time.Hour,
	}
}

// press sends a key and runs the command it produced, returning both.
func press(t *testing.T, m model, key tea.KeyMsg) (model, tea.Msg) {
	t.Helper()

	next, cmd := m.Update(key)
	m = next.(model)
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
}

func runes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
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

func TestViewRendersContainerRowWithoutPID(t *testing.T) {
	m, _ := testModel(t)
	m = m.applyScan(listenersMsg{listeners: []portscan.Listener{containerListener()}})

	view := m.View()
	for _, want := range []string{"8080", "zero-waste", "zero-waste-keycloak", "12h00m"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, " 0 ") {
		t.Fatalf("container row should not show PID 0:\n%s", view)
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

func TestApplyScanKeepsTheCursorBesideAVanishedRow(t *testing.T) {
	listeners := append(sampleListeners(), portscan.Listener{
		PID: 333, Port: 8080, Project: "docs", Command: "mkdocs serve", Uptime: time.Minute,
	})

	m, _ := testModel(t)
	m = m.applyScan(listenersMsg{listeners: listeners})
	m.table.SetCursor(2)

	// The selected process is killed; the cursor must stay beside it.
	m = m.applyScan(listenersMsg{listeners: listeners[:2]})

	selected, ok := m.selected()
	if !ok || selected.PID != 222 {
		t.Fatalf("selected = %+v (ok=%v), want PID 222, the row above the killed one", selected, ok)
	}
}

func TestSelectionWrapsAroundTheEnds(t *testing.T) {
	for _, keys := range []struct {
		name     string
		up, down tea.KeyMsg
	}{
		{"letters", runes("u"), runes("j")},
		{"arrows", tea.KeyMsg{Type: tea.KeyUp}, tea.KeyMsg{Type: tea.KeyDown}},
	} {
		t.Run(keys.name, func(t *testing.T) {
			m, _ := testModel(t)
			m = m.applyScan(listenersMsg{listeners: sampleListeners()})

			next, _ := m.Update(keys.up)
			m = next.(model)
			if selected, _ := m.selected(); selected.PID != 222 {
				t.Fatalf("up on the first row selected PID %d, want the last row (222)", selected.PID)
			}

			next, _ = m.Update(keys.down)
			m = next.(model)
			if selected, _ := m.selected(); selected.PID != 111 {
				t.Fatalf("down on the last row selected PID %d, want the first row (111)", selected.PID)
			}
		})
	}
}

func TestApplyScanReportsScanError(t *testing.T) {
	m, _ := testModel(t)
	m = m.applyScan(listenersMsg{err: errors.New("permission denied")})

	if m.statusKind != statusError || !strings.Contains(m.status, "permission denied") {
		t.Fatalf("status = %q (kind %v), want a scan error", m.status, m.statusKind)
	}
}

func TestKillKeySendsSIGTERMThenSIGKILL(t *testing.T) {
	m, calls := testModel(t)
	m = m.applyScan(listenersMsg{listeners: sampleListeners()})

	m, msg := press(t, m, runes("k"))
	if len(*calls) != 1 || (*calls)[0] != "TERM" {
		t.Fatalf("calls = %v, want one SIGTERM", *calls)
	}
	if result, ok := msg.(killResultMsg); !ok || !strings.Contains(result.action, "SIGTERM") {
		t.Fatalf("msg = %#v, want a SIGTERM result", msg)
	}

	// Second press within the escalation window.
	m, msg = press(t, m, runes("k"))
	if len(*calls) != 2 || (*calls)[1] != "KILL" {
		t.Fatalf("calls = %v, want SIGTERM then SIGKILL", *calls)
	}
	if result, ok := msg.(killResultMsg); !ok || !strings.Contains(result.action, "SIGKILL") {
		t.Fatalf("msg = %#v, want a SIGKILL result", msg)
	}
}

func TestEnterAndXAlsoKill(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, runes("x")} {
		m, calls := testModel(t)
		m = m.applyScan(listenersMsg{listeners: sampleListeners()})

		if _, msg := press(t, m, key); msg == nil {
			t.Fatalf("key %v produced no kill", key)
		}
		if len(*calls) != 1 {
			t.Fatalf("key %v: calls = %v, want one signal", key, *calls)
		}
	}
}

func TestSecondPressAfterWindowSendsSIGTERMAgain(t *testing.T) {
	m, calls := testModel(t)
	m = m.applyScan(listenersMsg{listeners: sampleListeners()})

	m, _ = press(t, m, runes("k"))

	base := m.now()
	m.now = func() time.Time { return base.Add(3 * time.Second) }

	m, _ = press(t, m, runes("k"))
	if len(*calls) != 2 || (*calls)[1] != "TERM" {
		t.Fatalf("calls = %v, want two SIGTERMs", *calls)
	}
}

func TestJAndUMoveTheSelection(t *testing.T) {
	m, _ := testModel(t)
	m = m.applyScan(listenersMsg{listeners: sampleListeners()})

	next, _ := m.Update(runes("j"))
	m = next.(model)
	if selected, _ := m.selected(); selected.PID != 222 {
		t.Fatalf("after j selected PID %d, want 222", selected.PID)
	}

	next, _ = m.Update(runes("u"))
	m = next.(model)
	if selected, _ := m.selected(); selected.PID != 111 {
		t.Fatalf("after u selected PID %d, want 111", selected.PID)
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

	next, _ := m.Update(runes("j"))
	m = next.(model)
	press(t, m, runes("k"))

	if killed != 222 {
		t.Fatalf("killed PID %d, want 222 (second row after moving down with j)", killed)
	}
}

func TestContainerRowIsStoppedThroughDockerNotSignalled(t *testing.T) {
	m, calls := testModel(t)
	engine := &fakeDocker{}
	m.docker = engine
	m = m.applyScan(listenersMsg{listeners: []portscan.Listener{containerListener()}})

	m, msg := press(t, m, runes("k"))
	if len(engine.calls) != 1 || engine.calls[0] != "stop:49a5e1d8e0e0" {
		t.Fatalf("docker calls = %v, want a stop", engine.calls)
	}
	if len(*calls) != 0 {
		t.Fatalf("signals = %v, want none: killing a container must not signal the host proxy", *calls)
	}
	if result, ok := msg.(killResultMsg); !ok || !strings.Contains(result.target, "zero-waste-keycloak") {
		t.Fatalf("msg = %#v, want a result naming the container", msg)
	}

	// Second press escalates to docker kill.
	m, _ = press(t, m, runes("k"))
	if len(engine.calls) != 2 || engine.calls[1] != "kill:49a5e1d8e0e0" {
		t.Fatalf("docker calls = %v, want stop then kill", engine.calls)
	}
}

func TestContainerStopErrorIsReported(t *testing.T) {
	m, _ := testModel(t)
	m.docker = &fakeDocker{stopErr: errors.New("no such container")}
	m = m.applyScan(listenersMsg{listeners: []portscan.Listener{containerListener()}})

	m, msg := press(t, m, runes("k"))
	next, _ := m.Update(msg)
	m = next.(model)

	if m.statusKind != statusError || !strings.Contains(m.status, "no such container") {
		t.Fatalf("status = %q (kind %v), want the docker error", m.status, m.statusKind)
	}
}

func TestQuitKey(t *testing.T) {
	m, _ := testModel(t)

	_, cmd := m.Update(runes("q"))
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("cmd produced %T, want tea.QuitMsg", msg)
	}
}
