// Package ui renders the interactive table of listening processes.
package ui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Mozolas/portadmin/internal/docker"
	"github.com/Mozolas/portadmin/internal/portscan"
)

const refreshInterval = 2 * time.Second

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("240"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

type listenersMsg struct {
	listeners []portscan.Listener
	err       error
}

type tickMsg time.Time

type killResultMsg struct {
	target string
	action string
	err    error
}

// dockerControl is the slice of the Docker API the UI needs; it is an
// interface so the model can be tested without an engine.
type dockerControl interface {
	Containers(ctx context.Context) ([]docker.Container, error)
	Stop(ctx context.Context, id string) error
	Kill(ctx context.Context, id string) error
}

type statusKind int

const (
	statusInfo statusKind = iota
	statusOK
	statusWarn
	statusError
)

type model struct {
	table      table.Model
	listeners  []portscan.Listener
	status     string
	statusKind statusKind

	lastKillKey string
	lastKillAt  time.Time

	docker  dockerControl
	version string

	// Injected so the model can be tested without signalling real processes.
	terminate func(int32) error
	kill      func(int32) error
	now       func() time.Time

	width  int
	height int
}

// Run starts the TUI.
func Run(version string) error {
	var engine dockerControl
	if client := docker.New(); client != nil {
		engine = client
	}

	m := newModel(engine)
	m.version = version
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newModel(engine dockerControl) model {
	t := table.New(
		table.WithColumns(columns(100)),
		table.WithFocused(true),
		table.WithHeight(15),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("231")).
		Background(lipgloss.Color("57")).
		Bold(true)
	t.SetStyles(s)

	// k is the kill key, so vertical movement uses j/u next to the arrows.
	t.KeyMap.LineUp = key.NewBinding(key.WithKeys("up", "u"), key.WithHelp("↑/u", "up"))
	t.KeyMap.LineDown = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	// u now moves one line up, so half-page scrolling keeps only its ctrl variant.
	t.KeyMap.HalfPageUp = key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "½ page up"))

	return model{
		table:      t,
		status:     "Scanning listening ports…",
		statusKind: statusInfo,
		width:      100,
		height:     24,
		docker:     engine,
		terminate:  portscan.Terminate,
		kill:       portscan.Kill,
		now:        time.Now,
	}
}

func columns(width int) []table.Column {
	const (
		portW   = 7
		project = 22
		pidW    = 8
		uptimeW = 9
		padding = 10 // cell padding used by the table renderer
	)

	command := width - portW - project - pidW - uptimeW - padding
	if command < 20 {
		command = 20
	}

	return []table.Column{
		{Title: "PORT", Width: portW},
		{Title: "PROJECT", Width: project},
		{Title: "COMMAND", Width: command},
		{Title: "PID", Width: pidW},
		{Title: "UPTIME", Width: uptimeW},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.scanCmd(), tickCmd())
}

func (m model) scanCmd() tea.Cmd {
	engine := m.docker
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var src portscan.ContainerSource
		if engine != nil {
			src = engine
		}
		listeners, err := portscan.ScanWithContainers(ctx, src)
		return listenersMsg{listeners: listeners, err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.table.SetColumns(columns(msg.Width))
		m.table.SetWidth(msg.Width)
		tableHeight := msg.Height - 7
		if tableHeight < 3 {
			tableHeight = 3
		}
		m.table.SetHeight(tableHeight)
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.scanCmd(), tickCmd())

	case listenersMsg:
		return m.applyScan(msg), nil

	case killResultMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("%s %s failed: %v", msg.action, msg.target, msg.err)
			m.statusKind = statusError
			return m, nil
		}
		m.status = fmt.Sprintf("%s %s.", msg.action, msg.target)
		m.statusKind = statusOK
		return m, m.scanCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.status = "Refreshing…"
			m.statusKind = statusInfo
			return m, m.scanCmd()
		case "enter", "k", "x":
			return m.killSelected()
		}

		if wrapped, ok := m.wrapSelection(msg); ok {
			return wrapped, nil
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// wrapSelection turns the rows into a ring: down on the last row jumps to the
// first and up on the first jumps to the last. The table alone only clamps.
func (m model) wrapSelection(msg tea.KeyMsg) (model, bool) {
	last := len(m.listeners) - 1
	if last < 1 {
		return m, false
	}

	switch {
	case key.Matches(msg, m.table.KeyMap.LineDown) && m.table.Cursor() == last:
		m.table.GotoTop()
	case key.Matches(msg, m.table.KeyMap.LineUp) && m.table.Cursor() == 0:
		m.table.GotoBottom()
	default:
		return m, false
	}
	return m, true
}

// applyScan replaces the table contents while keeping the cursor on the same
// process whenever it is still listening, and beside it once it is gone.
func (m model) applyScan(msg listenersMsg) model {
	if msg.err != nil {
		m.status = "Scan failed: " + msg.err.Error()
		m.statusKind = statusError
		return m
	}

	selected, hadSelection := m.selected()
	previous := m.table.Cursor()
	m.listeners = msg.listeners

	rows := make([]table.Row, 0, len(msg.listeners))
	cursor := -1
	for i, l := range msg.listeners {
		if hadSelection && l.PID == selected.PID && l.Port == selected.Port {
			cursor = i
		}
		rows = append(rows, table.Row{
			strconv.FormatUint(uint64(l.Port), 10),
			portscan.Truncate(displayProject(l), 22),
			portscan.Truncate(l.Command, commandWidth(m.table.Columns())),
			displayPID(l),
			portscan.FormatUptime(l.Uptime),
		})
	}

	m.table.SetRows(rows)
	if cursor < 0 {
		// The selected process is gone, killed or stopped on its own: stay on
		// the row above it instead of jumping back to the top of the table.
		cursor = previous - 1
	}
	m.table.SetCursor(cursor)

	if len(rows) == 0 {
		m.status = "No processes are listening on a TCP port."
		m.statusKind = statusInfo
	} else if m.statusKind == statusInfo {
		m.status = fmt.Sprintf("%d listening process(es).", len(rows))
	}
	return m
}

func commandWidth(cols []table.Column) int {
	if len(cols) < 3 {
		return 40
	}
	return cols[2].Width
}

func displayPID(l portscan.Listener) string {
	if l.PID == 0 {
		return "-"
	}
	return strconv.FormatInt(int64(l.PID), 10)
}

func displayProject(l portscan.Listener) string {
	if l.Project == "" {
		return "-"
	}
	return l.Project
}

func (m model) selected() (portscan.Listener, bool) {
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.listeners) {
		return portscan.Listener{}, false
	}
	return m.listeners[cursor], true
}

// killSelected stops the selected row: SIGTERM for a host process, docker stop
// for a container. Pressing again within two seconds escalates to SIGKILL.
func (m model) killSelected() (tea.Model, tea.Cmd) {
	target, ok := m.selected()
	if !ok {
		return m, nil
	}

	now := m.now()
	escalate := portscan.ShouldEscalate(m.lastKillKey, target.Key(), m.lastKillAt, now)

	if escalate {
		m.lastKillKey = ""
		m.lastKillAt = time.Time{}
	} else {
		m.lastKillKey = target.Key()
		m.lastKillAt = now
	}

	if target.IsContainer() {
		if escalate {
			m.status = fmt.Sprintf("Killing container %s…", target.ContainerName)
		} else {
			m.status = fmt.Sprintf("Stopping container %s… (press again within 2s to kill it)", target.ContainerName)
		}
		m.statusKind = statusWarn
		return m, m.stopContainerCmd(target, escalate)
	}

	if escalate {
		m.status = fmt.Sprintf("Sending SIGKILL to %s (PID %d)…", displayProject(target), target.PID)
	} else {
		m.status = fmt.Sprintf("Sending SIGTERM to %s (PID %d)… (press again within 2s for SIGKILL)", displayProject(target), target.PID)
	}
	m.statusKind = statusWarn
	return m, m.signalCmd(target, escalate)
}

func (m model) signalCmd(target portscan.Listener, escalate bool) tea.Cmd {
	label := fmt.Sprintf("%s (PID %d)", displayProject(target), target.PID)
	send, action := m.terminate, "SIGTERM sent to"
	if escalate {
		send, action = m.kill, "SIGKILL sent to"
	}

	return func() tea.Msg {
		return killResultMsg{target: label, action: action, err: send(target.PID)}
	}
}

func (m model) stopContainerCmd(target portscan.Listener, escalate bool) tea.Cmd {
	engine := m.docker
	label := fmt.Sprintf("container %s", target.ContainerName)
	id := target.ContainerID

	return func() tea.Msg {
		if engine == nil {
			return killResultMsg{target: label, action: "stop", err: fmt.Errorf("docker is not available")}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if escalate {
			return killResultMsg{target: label, action: "Killed", err: engine.Kill(ctx, id)}
		}
		return killResultMsg{target: label, action: "Stopped", err: engine.Stop(ctx, id)}
	}
}

func (m model) View() string {
	name := "portadmin"
	if m.version != "" {
		name += " " + m.version
	}
	title := titleStyle.Render(name) + headerStyle.Render("  ·  local ports at a glance")

	status := m.status
	switch m.statusKind {
	case statusOK:
		status = okStyle.Render(status)
	case statusWarn:
		status = warnStyle.Render(status)
	case statusError:
		status = errorStyle.Render(status)
	default:
		status = helpStyle.Render(status)
	}

	help := helpStyle.Render("↑/↓ or j/u move · k/enter/x kill (again within 2s = force) · r refresh · q quit")

	return fmt.Sprintf("%s\n\n%s\n\n%s\n%s\n", title, m.table.View(), status, help)
}
