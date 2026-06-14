package ui

import (
	"database/sql"
	"fmt"

	"quickcart-tui/db"

	"github.com/charmbracelet/lipgloss"
)

// DashboardModel is the stats overview screen
type DashboardModel struct {
	stats      db.Stats
	tokenStats db.TokenStats
	width      int
	height     int
	loaded     bool
	err        error
}

func NewDashboard(sqlDB *sql.DB, runsDB *sql.DB) DashboardModel {
	m := DashboardModel{}
	stats, err := db.GetStats(sqlDB)
	m.stats = stats
	m.err = err
	m.loaded = true
	if runsDB != nil {
		m.tokenStats, _ = db.GetTokenStats(runsDB)
	}
	return m
}

func (m DashboardModel) WithSize(w, h int) DashboardModel {
	m.width = w
	m.height = h
	return m
}

func (m DashboardModel) View() string {
	if m.err != nil {
		return StyleStatusErr.Render("Error loading stats: " + m.err.Error())
	}
	if !m.loaded {
		return StyleMuted.Render("Loading...")
	}

	s := m.stats
	t := m.tokenStats
	barW := 28
	col := m.width / 2
	if col < 40 {
		col = 40
	}

	// ── Sentiment section ──────────────────────────────────────────────────
	sentTitle := StyleSection.Render("  Sentiment Breakdown")
	negPct := pct(s.Negative, s.Total)
	posPct := pct(s.Positive, s.Total)
	neuPct := pct(s.Neutral, s.Total)

	sentRows := fmt.Sprintf(
		"  %s %s  %s  %3.0f%% (%d)\n"+
			"  %s %s  %s  %3.0f%% (%d)\n"+
			"  %s %s  %s  %3.0f%% (%d)",
		StyleNegative.Render("● Negative"),
		Bar(s.Negative, s.Total, barW, Red),
		StyleNegative.Render(fmt.Sprintf("%d", s.Negative)), negPct, s.Negative,
		StylePositive.Render("● Positive"),
		Bar(s.Positive, s.Total, barW, Green),
		StylePositive.Render(fmt.Sprintf("%d", s.Positive)), posPct, s.Positive,
		StyleNeutral.Render("● Neutral "),
		Bar(s.Neutral, s.Total, barW, Amber),
		StyleNeutral.Render(fmt.Sprintf("%d", s.Neutral)), neuPct, s.Neutral,
	)
	sentCard := StyleCard.Width(col - 4).Render(sentTitle + "\n\n" + sentRows)

	// ── Category section ───────────────────────────────────────────────────
	catTitle := StyleSection.Render("  Top Categories")
	catLines := ""
	cats := []string{"Delivery", "App Bug", "Staff/Support", "Billing", "Other"}
	catMap := make(map[string]int)
	for _, c := range s.Categories {
		catMap[c.Name] = c.Count
	}
	for i, cat := range cats {
		cnt := catMap[cat]
		rank := fmt.Sprintf("%d.", i+1)
		catLines += fmt.Sprintf("  %s %-14s %s %d\n",
			StyleMuted.Render(rank),
			StyleBold.Render(cat),
			Bar(cnt, s.Total, 20, Cyan),
			cnt,
		)
	}
	catCard := StyleCard.Width(col - 4).Render(catTitle + "\n\n" + catLines)

	// ── Summary metric cards ───────────────────────────────────────────────
	totalCard   := metricCard("Total Processed",  fmt.Sprintf("%d", s.Total),   Purple)
	droppedCard := metricCard("Rows Dropped",     fmt.Sprintf("%d", s.Dropped), Gray)
	flaggedCard := metricCard("Rating Conflicts", fmt.Sprintf("%d", s.Flagged), Amber)
	healthCard  := metricCard("Pipeline",         "✓ Complete",                  Green)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top,
		totalCard+"  ", droppedCard+"  ", flaggedCard+"  ", healthCard,
	)

	// ── Token utilization row ──────────────────────────────────────────────
	tokenRow := ""
	if t.TotalRuns > 0 {
		tokPerRow := 0
		if s.Total > 0 {
			tokPerRow = t.TotalTokens / s.Total
		}
		tpCard := metricCard("Prompt Tokens",    fmt.Sprintf("%d", t.TotalPrompt),      Cyan)
		tcCard := metricCard("Completion Tokens", fmt.Sprintf("%d", t.TotalCompletion),  Green)
		ttCard := metricCard("Total Tokens",     fmt.Sprintf("%d", t.TotalTokens),      Purple)
		teCard := metricCard("Efficiency",       fmt.Sprintf("%d tkns/row", tokPerRow), Amber)
		taCard := metricCard("API Calls",        fmt.Sprintf("%d", t.TotalAPICalls),    Gray)

		tokenTitle := StyleSection.Render("  AI Token Utilization") +
			StyleMuted.Render(fmt.Sprintf("  (across %d run(s))", t.TotalRuns))

		tokenRow = "\n" + tokenTitle + "\n" +
			lipgloss.JoinHorizontal(lipgloss.Top,
				tpCard+"  ", tcCard+"  ", ttCard+"  ", teCard+"  ", taCard,
			)
	}

	botRow := lipgloss.JoinHorizontal(lipgloss.Top, sentCard+"  ", catCard)

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		topRow,
		"",
		botRow,
		tokenRow,
	)
}

func metricCard(label, value string, color lipgloss.Color) string {
	v := lipgloss.NewStyle().Bold(true).Foreground(color).Render(value)
	l := StyleMuted.Render(label)
	return StyleCard.Width(18).Render(v + "\n" + l)
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}
