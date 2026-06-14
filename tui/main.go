package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"quickcart-tui/db"
	"quickcart-tui/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Tab definitions ──────────────────────────────────────────────────────────
const (
	tabDashboard = iota
	tabTable
	tabRunner
	tabReport
	tabHistory
	tabCount
)

var tabNames = []string{
	"  Dashboard ",
	"  Feedback  ",
	"  Run Pipeline ",
	"  Report ",
	"  History ",
}

// ── Root model ───────────────────────────────────────────────────────────────
type model struct {
	activeTab int
	width     int
	height    int
	sqlDB     *sql.DB
	runsDB    *sql.DB
	dashboard ui.DashboardModel
	table     ui.TableModel
	runner    ui.RunnerModel
	report    ui.ReportModel
	history   ui.HistoryModel
	spinner   spinner.Model
	loading   bool
	err       error
}

func newModel(sqlDB *sql.DB, runsDB *sql.DB, pythonPath, mainPath, reportPath, aiLogPath string) model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ui.Purple)

	return model{
		sqlDB:     sqlDB,
		runsDB:    runsDB,
		spinner:   sp,
		loading:   false,
		dashboard: ui.NewDashboard(sqlDB, runsDB),
		table:     ui.NewTable(sqlDB),
		runner:    ui.NewRunner(pythonPath, mainPath),
		report:    ui.NewReport(reportPath, aiLogPath),
		history:   ui.NewHistory(runsDB),
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.dashboard = m.dashboard.WithSize(m.width, m.height)
		m.table = m.table.WithSize(m.width, m.height)
		m.runner = m.runner.WithSize(m.width, m.height)
		m.report = m.report.WithSize(m.width, m.height)
		m.history = m.history.WithSize(m.width, m.height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "1":
			m.activeTab = tabDashboard
			return m, nil
		case "2":
			m.activeTab = tabTable
			return m, nil
		case "3":
			m.activeTab = tabRunner
			return m, nil
		case "4":
			m.activeTab = tabReport
			return m, nil
		case "5":
			m.activeTab = tabHistory
			return m, nil
		case "tab":
			m.activeTab = (m.activeTab + 1) % tabCount
			return m, nil
		case "shift+tab":
			m.activeTab = (m.activeTab - 1 + tabCount) % tabCount
			return m, nil
		}
	}

	// ── Always route runner messages regardless of active tab ─────────────
	// This keeps the cron countdown ticking and pipeline output streaming
	// even when the user is viewing a different tab.
	switch msg.(type) {
	case ui.ScheduleTickMsg, ui.PipelineLineMsg, ui.PipelineDoneMsg:
		var cmd tea.Cmd
		m.runner, cmd = m.runner.Update(msg)
		cmds = append(cmds, cmd)
		// If cron fires an auto-run while on another tab, switch to runner
		if _, isLine := msg.(ui.PipelineLineMsg); isLine && m.activeTab != tabRunner {
			// don't force switch — keep user where they are
		}
		return m, tea.Batch(cmds...)
	}

	// ── Delegate to active screen ──────────────────────────────────────────
	switch m.activeTab {
	case tabTable:
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)
	case tabRunner:
		var cmd tea.Cmd
		m.runner, cmd = m.runner.Update(msg)
		cmds = append(cmds, cmd)
	case tabReport:
		var cmd tea.Cmd
		m.report, cmd = m.report.Update(msg)
		cmds = append(cmds, cmd)
	case tabHistory:
		var cmd tea.Cmd
		m.history, cmd = m.history.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.err != nil {
		return ui.StyleStatusErr.Render("Fatal error: " + m.err.Error() + "\nPress q to quit.")
	}
	if m.height == 0 || m.width == 0 {
		return ""
	}

	header := m.renderHeader()
	tabs   := m.renderTabs()
	help   := m.renderHelp()

	// Measure chrome height AFTER rendering so we use the real line count.
	chromeH := lipgloss.Height(header) + lipgloss.Height(tabs) + lipgloss.Height(help)
	contentH := m.height - chromeH
	if contentH < 1 {
		contentH = 1
	}

	content := m.renderContent(contentH)

	// Simple concatenation — no JoinVertical so no extra newlines are inserted.
	return header + tabs + "\n" + content + "\n" + help
}

func (m model) renderHeader() string {
	brand := ui.StyleBrand.Render("⚡ QuickCart")
	sub   := ui.StyleSubtitle.Render(" Feedback Intelligence System")
	right := ui.StyleMuted.Render("q quit · tab switch")
	gap := m.width - lipgloss.Width(brand+sub) - lipgloss.Width(right) - 2
	if gap < 0 {
		gap = 0
	}
	spacer := lipgloss.NewStyle().Width(gap).Render("")
	// No trailing \n — let View() control line breaks
	return lipgloss.JoinHorizontal(lipgloss.Center, brand, sub, spacer, right)
}

func (m model) renderTabs() string {
	var tabs []string
	for i, name := range tabNames {
		if i == m.activeTab {
			tabs = append(tabs, ui.StyleActiveTab.Render(name))
		} else {
			tabs = append(tabs, ui.StyleTab.Render(name))
		}
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	return ui.StyleTabBar.Width(m.width).Render(row)
}

func (m model) renderContent(h int) string {
	w := m.width
	if w < 40 {
		w = 40
	}

	var content string
	switch m.activeTab {
	case tabDashboard:
		content = m.dashboard.View()
	case tabTable:
		content = m.table.View()
	case tabRunner:
		content = m.runner.View()
	case tabReport:
		content = m.report.View()
	case tabHistory:
		content = m.history.View()
	}

	// Pad short content and hard-clip overflow to exactly h lines.
	// Width(w) ensures every line (including blank padding) is w chars wide,
	// which physically overwrites any residual content from previous renders.
	return lipgloss.NewStyle().
		Width(w).
		Height(h).
		MaxHeight(h).
		Render(content)
}

func (m model) renderHelp() string {
	hints := "[1] Dashboard  [2] Feedback  [3] Run  [4] Report  [5] History  [tab] cycle  [q] quit"
	if m.activeTab == tabRunner {
		hints = "[enter] run  [p] parallel  [f] file  [d] default  [c] schedule  [x] off  [↑↓] scroll  [q] quit"
	}
	return ui.StyleHelp.Width(m.width).Render(hints)
}

// ── Entry point ──────────────────────────────────────────────────────────────
func main() {
	// Resolve project root: binary lives in project root, so exeDir = project root.
	// Fall back to cwd if that fails.
	exe, err := os.Executable()
	var projectRoot string
	if err != nil {
		projectRoot, _ = os.Getwd()
	} else {
		projectRoot = filepath.Dir(exe)
	}

	// Allow override via env (highest priority)
	if env := os.Getenv("QUICKCART_ROOT"); env != "" {
		projectRoot = env
	}

	// If DB doesn't exist at exeDir, try cwd (handles `go run .` case)
	if _, statErr := os.Stat(filepath.Join(projectRoot, "output", "feedback.db")); statErr != nil {
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			projectRoot = cwd
		}
	}

	dbPath := filepath.Join(projectRoot, "output", "feedback.db")
	reportPath := filepath.Join(projectRoot, "output", "summary_report.md")
	runsDBPath := filepath.Join(projectRoot, "output", "runs.db")
	aiLogPath  := filepath.Join(projectRoot, "ai_usage_log.md")
	pythonPath := filepath.Join(projectRoot, "venv", "bin", "python")
	mainPath   := filepath.Join(projectRoot, "main.py")

	// Open runs.db (open or create — runs.db may not exist yet)
	runsDB, err := db.Open(runsDBPath)
	if err != nil {
		// runs.db missing is fine — will be created on first pipeline run
		runsDB, _ = sql.Open("sqlite", runsDBPath)
	}

	// Open database
	sqlDB, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot open DB at %s: %v\n", dbPath, err)
		fmt.Fprintln(os.Stderr, "Make sure the pipeline has been run first.")
		os.Exit(1)
	}
	defer sqlDB.Close()

	m := newModel(sqlDB, runsDB, pythonPath, mainPath, reportPath, aiLogPath)

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "TUI error:", err)
		os.Exit(1)
	}
}
