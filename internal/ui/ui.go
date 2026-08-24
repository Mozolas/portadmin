// Package ui renders the interactive table of listening processes.
package ui

import (
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

	lastKillPID int32
	lastKillAt  time.Time

	// Injected so the model can be tested without signalling real processes.
	terminate func(int32) error
	kill      func(int32) error
	now       func() time.Time

	width  int
	height int
}

// Run starts the TUI.
func Run() error {
	m := newModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newModel() model {
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

	return model{
		table:      t,
		status:     "Scanning listening ports…",
		statusKind: statusInfo,
		width:      100,
		height:     24,
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
	return tea.Batch(scanCmd(), tickCmd())
}

func scanCmd() tea.Cmd {
	return func() tea.Msg {
		listeners, err := portscan.Scan()
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
		return m, tea.Batch(scanCmd(), tickCmd())

	case listenersMsg:
		return m.applyScan(msg), nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.status = "Refreshing…"
			m.statusKind = statusInfo
			return m, scanCmd()
		case "enter", "x":
			return m.killSelected()
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// applyScan replaces the table contents while keeping the cursor on the same
// process whenever it is still listening.
func (m model) applyScan(msg listenersMsg) model {
	if msg.err != nil {
		m.status = "Scan failed: " + msg.err.Error()
		m.statusKind = statusError
		return m
	}

	selected, hadSelection := m.selected()
	m.listeners = msg.listeners

	rows := make([]table.Row, 0, len(msg.listeners))
	cursor := 0
	for i, l := range msg.listeners {
		if hadSelection && l.PID == selected.PID && l.Port == selected.Port {
			cursor = i
		}
		rows = append(rows, table.Row{
			strconv.FormatUint(uint64(l.Port), 10),
			portscan.Truncate(displayProject(l), 22),
			portscan.Truncate(l.Command, commandWidth(m.table.Columns())),
			strconv.FormatInt(int64(l.PID), 10),
			portscan.FormatUptime(l.Uptime),
		})
	}

	m.table.SetRows(rows)
	if cursor < len(rows) {
		m.table.SetCursor(cursor)
	}

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

func (m model) killSelected() (tea.Model, tea.Cmd) {
	target, ok := m.selected()
	if !ok {
		return m, nil
	}

	now := m.now()
	if portscan.ShouldEscalate(m.lastKillPID, m.lastKillAt, target.PID, now) {
		if err := m.kill(target.PID); err != nil {
			m.status = fmt.Sprintf("SIGKILL to PID %d failed: %v", target.PID, err)
			m.statusKind = statusError
			return m, nil
		}
		m.status = fmt.Sprintf("SIGKILL sent to %s (PID %d, port %d).", displayProject(target), target.PID, target.Port)
		m.statusKind = statusOK
		m.lastKillPID = 0
		m.lastKillAt = time.Time{}
		return m, scanCmd()
	}

	if err := m.terminate(target.PID); err != nil {
		m.status = fmt.Sprintf("SIGTERM to PID %d failed: %v", target.PID, err)
		m.statusKind = statusError
		return m, nil
	}
	m.lastKillPID = target.PID
	m.lastKillAt = now
	m.status = fmt.Sprintf("SIGTERM sent to %s (PID %d) — press again within 2s for SIGKILL.", displayProject(target), target.PID)
	m.statusKind = statusWarn
	return m, scanCmd()
}

func (m model) View() string {
	title := titleStyle.Render("portadmin") + headerStyle.Render("  ·  local ports at a glance")

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

	help := helpStyle.Render("↑/↓ or j/k move · enter/x kill (again within 2s = SIGKILL) · r refresh · q quit")

	return fmt.Sprintf("%s\n\n%s\n\n%s\n%s\n", title, m.table.View(), status, help)
}
