package ui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

// ReportModel renders summary_report.md or ai_usage_log.md in a scrollable viewport
type ReportModel struct {
	vp         viewport.Model
	reportPath string
	aiLogPath  string
	showAILog  bool
	loaded     bool
	err        error
	width      int
	height     int
	renderer   *glamour.TermRenderer
}

func NewReport(reportPath, aiLogPath string) ReportModel {
	vp := viewport.New(80, 30)
	m := ReportModel{
		vp:         vp,
		reportPath: reportPath,
		aiLogPath:  aiLogPath,
	}
	m.load()
	return m
}

func (m *ReportModel) initRenderer() {
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(m.vp.Width),
	)
	if err != nil {
		// Fallback to auto style
		r, _ = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(m.vp.Width),
		)
	}
	m.renderer = r
}

func (m *ReportModel) load() {
	path := m.reportPath
	if m.showAILog {
		path = m.aiLogPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		m.err = err
		m.vp.SetContent(StyleStatusErr.Render("Could not read file: " + err.Error()))
		return
	}

	if m.renderer == nil {
		m.initRenderer()
	}

	rendered, err := m.renderer.Render(string(data))
	if err != nil {
		// Fallback to plain text on render error
		m.vp.SetContent(string(data))
	} else {
		m.vp.SetContent(rendered)
	}

	m.vp.GotoTop()
	m.loaded = true
	m.err = nil
}

func (m ReportModel) WithSize(w, h int) ReportModel {
	m.width = w
	m.height = h
	m.vp.Width = w - 6
	m.vp.Height = h - 8
	// Re-init renderer with new width for proper word wrap
	m.renderer = nil
	m.load()
	return m
}

func (m ReportModel) Update(msg tea.Msg) (ReportModel, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			m.renderer = nil // force re-render
			m.load()
		case "a":
			m.showAILog = !m.showAILog
			m.renderer = nil
			m.load()
		}
	}
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m ReportModel) View() string {
	// Title bar
	title := "  📊 Summary Report"
	if m.showAILog {
		title = "  🤖 AI Usage Log"
	}
	titleBar := StyleSection.Render(title) +
		StyleMuted.Render("    [a] toggle  [r] reload  [↑↓] scroll")

	// Scroll info
	pct := 0
	if m.vp.TotalLineCount() > 0 {
		pct = int(m.vp.ScrollPercent() * 100)
	}
	scrollBar := Bar(pct, 100, 24, Purple)
	scrollInfo := StyleMuted.Render(
		fmt.Sprintf("  line %d/%d  ", m.vp.YOffset, m.vp.TotalLineCount()),
	) + scrollBar + StyleMuted.Render(fmt.Sprintf("  %d%%", pct))

	return "\n" +
		titleBar + "\n\n" +
		m.vp.View() + "\n\n" +
		scrollInfo
}
