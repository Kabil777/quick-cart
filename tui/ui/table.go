package ui

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"quickcart-tui/db"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ExportDoneMsg carries the result of a CSV export
type ExportDoneMsg struct {
	Path string
	Err  error
	Rows int
}

var sentimentFilters = []string{"all", "negative", "positive", "neutral"}
var categoryFilters = []string{"All", "Billing", "App Bug", "Delivery", "Staff/Support", "Other"}

// matchedCol records which column matched for display
type matchedRow struct {
	row     db.Feedback
	matchIn string // comma-separated column names that matched
}

// ── Detail Popup ──────────────────────────────────────────────────────────────

type popupModel struct {
	vp      viewport.Model
	row     db.Feedback
	visible bool
	width   int
	height  int
}

func newPopup(w, h int) popupModel {
	vp := viewport.New(w-8, h-10)
	return popupModel{vp: vp, width: w, height: h}
}

func (p *popupModel) open(r db.Feedback) {
	p.row = r
	p.visible = true
	p.vp.SetContent(p.render())
	p.vp.GotoTop()
}

func (p *popupModel) close() {
	p.visible = false
}

func (p popupModel) render() string {
	r := p.row
	flags := ""
	if r.RatingContradiction {
		flags += StyleStatusErr.Render("  ⚠ Rating contradicts text") + "\n"
	}
	if r.TimestampMissing {
		flags += StyleMuted.Render("  ⚠ Timestamp missing") + "\n"
	}
	ratingStr := "—"
	if r.Rating > 0 {
		ratingStr = fmt.Sprintf("%.0f ★", r.Rating)
	}
	sentBadge := lipgloss.NewStyle().Bold(true).Foreground(White).
		Background(SentimentColor(r.Sentiment)).Padding(0, 1).
		Render(strings.ToUpper(r.Sentiment))
	catBadge := lipgloss.NewStyle().Bold(true).Foreground(White).
		Background(Purple).Padding(0, 1).Render(r.Category)
	divider := StyleMuted.Render(strings.Repeat("─", p.vp.Width-2))

	content := strings.Join([]string{
		sentBadge + "  " + catBadge, "",
		StyleSection.Render("Metadata"),
		fmt.Sprintf("  %s %s", StyleMuted.Render("ID:       "), StyleBold.Render(r.ID)),
		fmt.Sprintf("  %s %s", StyleMuted.Render("Source:   "), r.Source),
		fmt.Sprintf("  %s %s", StyleMuted.Render("Rating:   "), ratingStr),
		fmt.Sprintf("  %s %s", StyleMuted.Render("Timestamp:"), func() string {
			if r.Timestamp == "" {
				return StyleMuted.Render("(missing)")
			}
			return r.Timestamp
		}()),
		"", flags, divider, "",
		StyleSection.Render("AI Summary"),
		"  " + StyleAccent.Render(r.Summary), "",
		divider, "",
		StyleSection.Render("Full Feedback Text"),
		wordWrap("  "+r.FeedbackText, p.vp.Width-4),
	}, "\n")
	return content
}

func wordWrap(text string, width int) string {
	words := strings.Fields(text)
	if width <= 0 || len(words) == 0 {
		return text
	}
	var lines []string
	line := ""
	for _, w := range words {
		if len(line)+len(w)+1 > width {
			if line != "" {
				lines = append(lines, line)
			}
			line = w
		} else {
			if line == "" {
				line = w
			} else {
				line += " " + w
			}
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (p popupModel) View() string {
	if !p.visible {
		return ""
	}
	title := StyleBrand.Render("  Feedback Detail  ") +
		StyleMuted.Render("  esc/q close  ↑↓ scroll")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(Purple).
		Padding(1, 2).Width(p.width - 6).
		Render(title + "\n\n" + p.vp.View())
	topPad := (p.height - lipgloss.Height(box)) / 2
	if topPad < 0 {
		topPad = 0
	}
	return strings.Repeat("\n", topPad) + box
}

// ── Search engine ─────────────────────────────────────────────────────────────

type searchMode int

const (
	modeLiteral searchMode = iota
	modeRegex
)

// applySearch filters allRows using the current pattern across all columns.
// Returns matched rows with the matching column name(s).
func applySearch(allRows []db.Feedback, pattern string, mode searchMode) ([]matchedRow, error) {
	if pattern == "" {
		out := make([]matchedRow, len(allRows))
		for i, r := range allRows {
			out[i] = matchedRow{row: r}
		}
		return out, nil
	}

	var re *regexp.Regexp
	var err error

	if mode == modeRegex {
		re, err = regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
	} else {
		// Literal: escape regex metacharacters
		re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(pattern))
	}

	type colPair struct {
		name string
		val  string
	}

	var result []matchedRow
	for _, r := range allRows {
		cols := []colPair{
			{"id", r.ID},
			{"sentiment", r.Sentiment},
			{"category", r.Category},
			{"source", r.Source},
			{"timestamp", r.Timestamp},
			{"summary", r.Summary},
			{"text", r.FeedbackText},
		}
		var matched []string
		for _, c := range cols {
			if re.MatchString(c.val) {
				matched = append(matched, c.name)
			}
		}
		if len(matched) > 0 {
			result = append(result, matchedRow{
				row:     r,
				matchIn: strings.Join(matched, ","),
			})
		}
	}
	return result, nil
}

// highlightMatch wraps the first occurrence of pattern in a string with styling
func highlightMatch(s, pattern string, mode searchMode) string {
	if pattern == "" {
		return Truncate(s, 48)
	}
	var re *regexp.Regexp
	if mode == modeRegex {
		re, _ = regexp.Compile("(?i)" + pattern)
	} else {
		re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(pattern))
	}
	if re == nil {
		return Truncate(s, 48)
	}
	loc := re.FindStringIndex(s)
	if loc == nil {
		return Truncate(s, 48)
	}
	// Keep context around the match
	start, end := loc[0], loc[1]
	prefix := Truncate(s[:start], 20)
	match := lipgloss.NewStyle().Background(Amber).Foreground(lipgloss.Color("#000")).Bold(true).
		Render(s[start:end])
	suffix := Truncate(s[end:], 20)
	return prefix + match + suffix
}

// ── TableModel ────────────────────────────────────────────────────────────────

type TableModel struct {
	sqlDB         *sql.DB
	table         table.Model
	search        textinput.Model
	popup         popupModel
	sentIdx       int
	catIdx        int
	searchMode    searchMode
	searchFocused bool
	allRows       []db.Feedback // full set (SQL filtered only)
	filtered      []matchedRow  // after Go-side pattern filter
	width         int
	height        int
	status        string
	searchErr     error
	exportStatus  string // last export result message
}

func NewTable(sqlDB *sql.DB) TableModel {
	ti := textinput.New()
	ti.Placeholder = "Pattern search all columns… (ctrl+r: regex mode)"
	ti.CharLimit = 120

	cols := []table.Column{
		{Title: "ID",       Width: 5},
		{Title: "Sent.",    Width: 10},
		{Title: "Category", Width: 13},
		{Title: "Source",   Width: 12},
		{Title: "Match in", Width: 10},
		{Title: "Summary",  Width: 42},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	ts := table.DefaultStyles()
	ts.Header = ts.Header.Bold(true).Foreground(Cyan).BorderForeground(Border)
	ts.Selected = ts.Selected.Foreground(White).Background(Purple).Bold(true)
	t.SetStyles(ts)

	m := TableModel{
		sqlDB:  sqlDB,
		table:  t,
		search: ti,
		popup:  newPopup(120, 40),
	}
	m.reloadAll()
	return m
}

// reloadAll fetches from DB (SQL filters only) then applies Go-side search
func (m *TableModel) reloadAll() {
	rows, err := db.GetAllFeedback(
		m.sqlDB,
		sentimentFilters[m.sentIdx],
		categoryFilters[m.catIdx],
		2000,
	)
	if err != nil {
		m.status = "DB error: " + err.Error()
		return
	}
	m.allRows = rows
	m.applyFilter()
}

func (m *TableModel) applyFilter() {
	pattern := m.search.Value()
	matched, err := applySearch(m.allRows, pattern, m.searchMode)
	m.searchErr = err
	if err != nil {
		m.filtered = nil
		m.status = StyleStatusErr.Render("⚠ " + err.Error())
		m.table.SetRows(nil)
		return
	}
	m.filtered = matched
	m.status = fmt.Sprintf("%d / %d rows", len(matched), len(m.allRows))

	tRows := make([]table.Row, len(matched))
	for i, mr := range matched {
		r := mr.row
		matchLabel := StyleMuted.Render("—")
		if mr.matchIn != "" {
			matchLabel = StyleAccent.Render(mr.matchIn)
		}
		summary := highlightMatch(r.Summary, pattern, m.searchMode)
		tRows[i] = table.Row{r.ID, r.Sentiment, r.Category, r.Source, matchLabel, summary}
	}
	m.table.SetRows(tRows)
}

func (m TableModel) WithSize(w, h int) TableModel {
	m.width = w
	m.height = h
	// Reserve lines: filterBar(1) + searchBox(0-3) + gap(1) + detail(0-3) + gap(1) + help(1) = ~7-11
	// Use h-12 so table has breathing room regardless of optional elements
	tableH := h - 12
	if tableH < 5 {
		tableH = 5
	}
	m.table.SetHeight(tableH)
	m.popup = newPopup(w, h)
	return m
}

func (m TableModel) Update(msg tea.Msg) (TableModel, tea.Cmd) {
	var cmd tea.Cmd

	// Popup intercepts all keys
	if m.popup.visible {
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "esc", "q", "enter":
				m.popup.close()
				return m, nil
			}
		}
		m.popup.vp, cmd = m.popup.vp.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.searchFocused {
			switch msg.String() {
			case "enter", "esc":
				m.searchFocused = false
				m.search.Blur()
			case "ctrl+r":
				// Toggle regex / literal mode
				if m.searchMode == modeLiteral {
					m.searchMode = modeRegex
				} else {
					m.searchMode = modeLiteral
				}
				m.applyFilter()
			default:
				m.search, cmd = m.search.Update(msg)
				m.applyFilter()
			}
			return m, cmd
		}

		// Normal mode
		switch msg.String() {
		case "enter":
			if idx := m.table.Cursor(); idx >= 0 && idx < len(m.filtered) {
				m.popup.open(m.filtered[idx].row)
			}
		case "/":
			m.searchFocused = true
			m.search.Focus()
			return m, textinput.Blink
		case "ctrl+r":
			if m.searchMode == modeLiteral {
				m.searchMode = modeRegex
			} else {
				m.searchMode = modeLiteral
			}
			m.applyFilter()
		case "e":
			// Export current filtered view to timestamped CSV
			rows := make([]db.Feedback, len(m.filtered))
			for i, mr := range m.filtered {
				rows[i] = mr.row
			}
			return m, exportCSV(rows)
		case "s":
			m.sentIdx = (m.sentIdx + 1) % len(sentimentFilters)
			m.reloadAll()
		case "c":
			m.catIdx = (m.catIdx + 1) % len(categoryFilters)
			m.reloadAll()
		case "r":
			m.search.Reset()
			m.sentIdx = 0
			m.catIdx = 0
			m.searchMode = modeLiteral
			m.reloadAll()
		default:
			m.table, cmd = m.table.Update(msg)
		}

	case ExportDoneMsg:
		if msg.Err != nil {
			m.exportStatus = StyleStatusErr.Render(fmt.Sprintf("  ✗ Export failed: %s", msg.Err.Error()))
		} else {
			m.exportStatus = StyleStatusOK.Render(fmt.Sprintf("  ✓ Exported %d rows → %s", msg.Rows, msg.Path))
		}
	}
	return m, cmd
}

// exportCSV writes the current filtered rows to a timestamped CSV file
func exportCSV(rows []db.Feedback) tea.Cmd {
	return func() tea.Msg {
		if len(rows) == 0 {
			return ExportDoneMsg{Err: fmt.Errorf("no rows to export")}
		}
		_ = os.MkdirAll("output", 0755)
		path := fmt.Sprintf("output/export_%s.csv", time.Now().Format("2006-01-02_15-04-05"))
		f, err := os.Create(path)
		if err != nil {
			return ExportDoneMsg{Err: err}
		}
		defer f.Close()
		w := csv.NewWriter(f)
		_ = w.Write([]string{"id", "timestamp", "source", "rating", "sentiment", "category", "summary", "feedback_text", "rating_contradiction", "timestamp_missing"})
		for _, r := range rows {
			rc := "0"
			if r.RatingContradiction {
				rc = "1"
			}
			tm := "0"
			if r.TimestampMissing {
				tm = "1"
			}
			_ = w.Write([]string{
				r.ID, r.Timestamp, r.Source,
				fmt.Sprintf("%.0f", r.Rating),
				r.Sentiment, r.Category, r.Summary, r.FeedbackText, rc, tm,
			})
		}
		w.Flush()
		return ExportDoneMsg{Path: path, Rows: len(rows)}
	}
}

func (m TableModel) View() string {
	if m.popup.visible {
		return m.popup.View()
	}

	var sb strings.Builder

	// ── Mode badge ─────────────────────────────────────────────────────────
	modeBadge := lipgloss.NewStyle().
		Foreground(White).Background(Gray).Padding(0, 1).Render("LITERAL")
	if m.searchMode == modeRegex {
		modeBadge = lipgloss.NewStyle().
			Foreground(White).Background(Purple).Padding(0, 1).Render("REGEX")
	}

	// ── Filter bar ─────────────────────────────────────────────────────────
	sentColor := SentimentColor(sentimentFilters[m.sentIdx])
	sentLabel := lipgloss.NewStyle().Foreground(sentColor).Bold(true).
		Render(sentimentFilters[m.sentIdx])
	catLabel := StyleAccent.Render(categoryFilters[m.catIdx])
	filterBar := fmt.Sprintf("  %s  Sentiment: [%s]   Category: [%s]   %s",
		modeBadge, sentLabel, catLabel, StyleMuted.Render(m.status))
	sb.WriteString(filterBar + "\n")

	// ── Search box (only when active or has value) ──────────────────────────
	if m.searchFocused || m.search.Value() != "" {
		inputStyle := StyleInput.Width(m.width - 6)
		if m.searchErr != nil {
			inputStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).BorderForeground(Red).
				Padding(0, 1).Width(m.width - 6)
		}
		sb.WriteString(inputStyle.Render("  🔍 "+m.search.View()) + "\n")
		if m.searchErr != nil {
			sb.WriteString(StyleStatusErr.Render("  "+m.searchErr.Error()) + "\n")
		}
	}

	// ── Table ──────────────────────────────────────────────────────────────
	sb.WriteString(m.table.View() + "\n")

	// ── Detail strip (only when a row is selected) ─────────────────────────
	if idx := m.table.Cursor(); idx >= 0 && idx < len(m.filtered) {
		r := m.filtered[idx].row
		matchStr := ""
		if m.filtered[idx].matchIn != "" {
			matchStr = "  matched: " + StyleAccent.Render(m.filtered[idx].matchIn)
		}
		flags := ""
		if r.RatingContradiction {
			flags = "  " + StyleStatusErr.Render("⚠ rating conflict")
		}
		detail := StyleCard.Width(m.width - 4).Render(
			StyleBold.Render("ID "+r.ID)+" · "+
				SentimentStyle(r.Sentiment).Render(r.Sentiment)+" · "+
				StyleAccent.Render(r.Category)+flags+matchStr+"\n"+
				StyleMuted.Render("↳ ")+Truncate(r.Summary, m.width-20),
		)
		sb.WriteString(detail + "\n")
	}

	// ── Export status (only when set) ──────────────────────────────────────
	if m.exportStatus != "" {
		sb.WriteString(m.exportStatus + "\n")
	}

	// ── Help bar ───────────────────────────────────────────────────────────
	sb.WriteString(StyleHelp.Render(
		"  [/] search  [ctrl+r] regex  [s] sentiment  [c] category  [r] reset  [e] export  [enter] detail  [↑↓] nav",
	))

	return sb.String()
}
