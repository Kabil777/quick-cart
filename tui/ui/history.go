package ui

import (
	"database/sql"
	"fmt"
	"strings"

	"quickcart-tui/db"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HistoryModel shows all past pipeline runs from runs.db
type HistoryModel struct {
	runsDB *sql.DB
	runs   []db.PipelineRun
	cursor int
	width  int
	height int
	err    error
}

func NewHistory(runsDB *sql.DB) HistoryModel {
	m := HistoryModel{runsDB: runsDB}
	m.reload()
	return m
}

func (m *HistoryModel) reload() {
	runs, err := db.GetRuns(m.runsDB, 50)
	m.runs = runs
	m.err = err
}

func (m HistoryModel) WithSize(w, h int) HistoryModel {
	m.width = w
	m.height = h
	return m
}

func (m HistoryModel) Update(msg tea.Msg) (HistoryModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.runs)-1 {
				m.cursor++
			}
		case "r":
			m.reload()
		}
	}
	return m, nil
}

func (m HistoryModel) View() string {
	if m.err != nil {
		return "\n" + StyleCard.Width(m.width-4).Render(
			StyleStatusErr.Render("Could not load run history: "+m.err.Error()) + "\n\n" +
				StyleMuted.Render("Run the pipeline at least once to see history here.\n") +
				StyleMuted.Render("  cd quickcart-feedback && source venv/bin/activate && python main.py"),
		)
	}

	if len(m.runs) == 0 {
		return "\n" + StyleCard.Width(m.width-4).Render(
			StyleSection.Render("No runs recorded yet") + "\n\n" +
				StyleMuted.Render("Go to [3] Run Pipeline tab and press Enter to start.\n") +
				StyleMuted.Render("Each run will be logged here automatically."),
		)
	}

	// ── Run list (left panel) ─────────────────────────────────────────────────
	listLines := []string{StyleSection.Render("  Run History  ") + StyleMuted.Render(fmt.Sprintf("(%d runs)", len(m.runs)))}
	listLines = append(listLines, StyleMuted.Render(strings.Repeat("─", 36)))

	for i, r := range m.runs {
		status := statusBadge(r.Status)
		ts := r.StartedAt
		if len(ts) > 16 {
			ts = ts[:16]
		}
		modeIcon := "→"
		if r.Mode == "parallel" {
			modeIcon = "⚡"
		}
		line := fmt.Sprintf("  #%-3d %s %s %s %s",
			r.RunID,
			status,
			modeIcon,
			StyleMuted.Render(ts),
			StyleMuted.Render(fmt.Sprintf("%d rows", r.RowsEnriched)),
		)
		if i == m.cursor {
			line = lipgloss.NewStyle().Background(Purple).Foreground(White).Render(line + strings.Repeat(" ", 2))
		}
		listLines = append(listLines, line)
	}

	listPanel := StyleCard.Width(42).Height(m.height - 8).Render(
		strings.Join(listLines, "\n"),
	)

	// ── Detail panel (right) ─────────────────────────────────────────────────
	detailPanel := ""
	if m.cursor < len(m.runs) {
		r := m.runs[m.cursor]
		detailPanel = renderRunDetail(r, m.width-50)
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, listPanel+"  ", detailPanel)
	help := StyleHelp.Render("  [↑↓/j/k] navigate runs  [r] refresh  [1-5] switch tabs")

	return lipgloss.JoinVertical(lipgloss.Left, "", body, "", help)
}

func statusBadge(status string) string {
	switch status {
	case "complete":
		return lipgloss.NewStyle().Foreground(White).Background(Green).Padding(0, 1).Render("✓")
	case "running":
		return lipgloss.NewStyle().Foreground(White).Background(Amber).Padding(0, 1).Render("⏳")
	case "failed":
		return lipgloss.NewStyle().Foreground(White).Background(Red).Padding(0, 1).Render("✗")
	default:
		return lipgloss.NewStyle().Foreground(White).Background(Gray).Padding(0, 1).Render("?")
	}
}

func renderRunDetail(r db.PipelineRun, width int) string {
	if width < 20 {
		width = 40
	}

	modeLabel := r.Mode
	if r.Mode == "parallel" {
		modeLabel = "⚡ parallel"
	} else if r.Mode == "sequential" {
		modeLabel = "→ sequential"
	}

	// Duration calculation
	duration := "—"
	if r.CompletedAt != "" && r.StartedAt != "" {
		duration = "completed at " + r.CompletedAt
	}

	// Sentiment mini-bar
	total := r.Negative + r.Positive + r.Neutral
	sentBar := ""
	if total > 0 {
		sentBar = "\n\n" + StyleSection.Render("  Sentiment") + "\n" +
			fmt.Sprintf("  %s %s %d (%.0f%%)\n",
				StyleNegative.Render("● Neg"),
				Bar(r.Negative, total, 18, Red),
				r.Negative, float64(r.Negative)/float64(total)*100) +
			fmt.Sprintf("  %s %s %d (%.0f%%)\n",
				StylePositive.Render("● Pos"),
				Bar(r.Positive, total, 18, Green),
				r.Positive, float64(r.Positive)/float64(total)*100) +
			fmt.Sprintf("  %s %s %d (%.0f%%)",
				StyleNeutral.Render("● Neu"),
				Bar(r.Neutral, total, 18, Amber),
				r.Neutral, float64(r.Neutral)/float64(total)*100)
	}

	// Token utilization section
	tokenStr := ""
	if r.TokensTotal > 0 {
		tokPerRow := 0
		if r.RowsEnriched > 0 {
			tokPerRow = r.TokensTotal / r.RowsEnriched
		}
		tokenStr = "\n\n" + StyleSection.Render("  Token Utilization") + "\n" +
			fmt.Sprintf("  %s %s\n",
				StyleMuted.Render("Prompt:     "),
				lipgloss.NewStyle().Foreground(Cyan).Render(fmt.Sprintf("%d", r.TokensPrompt))) +
			fmt.Sprintf("  %s %s\n",
				StyleMuted.Render("Completion: "),
				lipgloss.NewStyle().Foreground(Green).Render(fmt.Sprintf("%d", r.TokensCompletion))) +
			fmt.Sprintf("  %s %s\n",
				StyleMuted.Render("Total:      "),
				StyleBold.Render(fmt.Sprintf("%d", r.TokensTotal))) +
			StyleMuted.Render(strings.Repeat("─", 26)) + "\n" +
			fmt.Sprintf("  %s %d\n",
				StyleMuted.Render("API Calls:  "), r.APICalls) +
			fmt.Sprintf("  %s %s\n",
				StyleMuted.Render("Retries:    "),
				func() string {
					if r.Retries > 0 {
						return StyleStatusErr.Render(fmt.Sprintf("%d", r.Retries))
					}
					return StyleStatusOK.Render("0")
				}()) +
			fmt.Sprintf("  %s %s",
				StyleMuted.Render("Efficiency: "),
				StyleAccent.Render(fmt.Sprintf("%d tkns/row", tokPerRow)))
	} else {
		tokenStr = "\n\n" + StyleMuted.Render("  No token data (run predates tracking)")
	}

	noteStr := ""
	if r.Notes != "" {
		noteStr = "\n\n" + StyleStatusErr.Render("  Error: "+Truncate(r.Notes, width-10))
	}

	content := fmt.Sprintf(
		"%s  %s\n\n"+
			StyleSection.Render("  Run Details")+"\n"+
			"  %s Run #%d\n"+
			"  %s %s\n"+
			"  %s %s\n"+
			"  %s %s\n\n"+
			StyleSection.Render("  Row Counts")+"\n"+
			"  %s %d\n"+
			"  %s %d\n"+
			"  %s %d\n"+
			"  %s %d",
		statusBadge(r.Status),
		lipgloss.NewStyle().Bold(true).Foreground(Cyan).Render(modeLabel),
		StyleMuted.Render("ID:       "), r.RunID,
		StyleMuted.Render("Started:  "), r.StartedAt,
		StyleMuted.Render("Status:   "), r.Status,
		StyleMuted.Render("Duration: "), duration,
		StyleMuted.Render("Raw:      "), r.RowsRaw,
		StyleMuted.Render("Cleaned:  "), r.RowsCleaned,
		StyleMuted.Render("Dropped:  "), r.RowsDropped,
		StyleMuted.Render("Enriched: "), r.RowsEnriched,
	) + sentBar + tokenStr + noteStr


	return StyleCard.Width(width).Render(content)
}
