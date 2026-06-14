package ui

import "github.com/charmbracelet/lipgloss"

var (
	// Palette
	Purple  = lipgloss.Color("#7C3AED")
	Cyan    = lipgloss.Color("#06B6D4")
	Red     = lipgloss.Color("#EF4444")
	Green   = lipgloss.Color("#10B981")
	Amber   = lipgloss.Color("#F59E0B")
	Gray    = lipgloss.Color("#6B7280")
	White   = lipgloss.Color("#F3F4F6")
	Border  = lipgloss.Color("#374151")
	Surface = lipgloss.Color("#1E1E2E")

	// Base
	StyleBase = lipgloss.NewStyle().Foreground(White)
	StyleBold = lipgloss.NewStyle().Bold(true).Foreground(White)
	StyleMuted = lipgloss.NewStyle().Foreground(Gray)
	StyleAccent = lipgloss.NewStyle().Foreground(Cyan).Bold(true)

	// App title / brand
	StyleBrand = lipgloss.NewStyle().
			Bold(true).
			Foreground(White).
			Background(Purple).
			Padding(0, 2)

	StyleSubtitle = lipgloss.NewStyle().Foreground(Gray).Italic(true)

	// Tabs
	StyleTab = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(Gray)

	StyleActiveTab = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(Purple).
			Bold(true).
			Underline(true)

	StyleTabBar = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(Border)

	// Cards
	StyleCard = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Border).
			Padding(0, 1)

	StyleCardHighlight = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(Purple).
				Padding(0, 1)

	// Sentiment
	StyleNegative = lipgloss.NewStyle().Foreground(Red).Bold(true)
	StylePositive = lipgloss.NewStyle().Foreground(Green).Bold(true)
	StyleNeutral  = lipgloss.NewStyle().Foreground(Amber).Bold(true)

	// Section header
	StyleSection = lipgloss.NewStyle().
			Bold(true).
			Foreground(Cyan).
			MarginBottom(1)

	// Help bar
	StyleHelp = lipgloss.NewStyle().
			Foreground(Gray).
			Padding(0, 1)

	// Input box
	StyleInput = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(Purple).
			Padding(0, 1)

	// Status bar
	StyleStatusOK  = lipgloss.NewStyle().Foreground(Green).Bold(true)
	StyleStatusErr = lipgloss.NewStyle().Foreground(Red).Bold(true)
	StyleStatusRun = lipgloss.NewStyle().Foreground(Amber).Bold(true)
)

func SentimentStyle(s string) lipgloss.Style {
	switch s {
	case "negative":
		return StyleNegative
	case "positive":
		return StylePositive
	default:
		return StyleNeutral
	}
}

func SentimentIcon(s string) string {
	switch s {
	case "negative":
		return "●"
	case "positive":
		return "●"
	default:
		return "●"
	}
}

func SentimentColor(s string) lipgloss.Color {
	switch s {
	case "negative":
		return Red
	case "positive":
		return Green
	default:
		return Amber
	}
}

// Bar renders a text progress bar
func Bar(value, total, width int, color lipgloss.Color) string {
	if total == 0 {
		return ""
	}
	filled := int(float64(width) * float64(value) / float64(total))
	if filled > width {
		filled = width
	}
	bar := lipgloss.NewStyle().Foreground(color).Render(repeatStr("█", filled))
	empty := lipgloss.NewStyle().Foreground(Border).Render(repeatStr("░", width-filled))
	return bar + empty
}

func repeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

func Truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
