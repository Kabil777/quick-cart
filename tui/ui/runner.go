package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Pipeline state ────────────────────────────────────────────────────────────

type PipelineState int

const (
	StateIdle PipelineState = iota
	StateRunning
	StateDone
	StateError
)

// ── Cron intervals ────────────────────────────────────────────────────────────

type CronInterval int

const (
	CronManual CronInterval = iota
	Cron30Min
	Cron1Hr
	Cron6Hr
	Cron12Hr
	Cron24Hr
)

var cronLabels = []string{"Manual", "30 min", "1 hour", "6 hours", "12 hours", "24 hours"}
var cronDurations = []time.Duration{
	0,
	30 * time.Minute,
	1 * time.Hour,
	6 * time.Hour,
	12 * time.Hour,
	24 * time.Hour,
}

// ── Messages ──────────────────────────────────────────────────────────────────

type PipelineLineMsg string
type PipelineDoneMsg struct{ Err error }
type ScheduleTickMsg time.Time  // fired by the cron ticker
type CronFireMsg     struct{}   // triggers auto-run

// ── RunnerModel ───────────────────────────────────────────────────────────────

type RunnerModel struct {
	// pipeline
	state      PipelineState
	vp         viewport.Model
	sp         spinner.Model
	lines      []string
	pythonPath string
	mainPath   string
	parallel   bool
	outputCh   chan string

	// csv file input
	csvInput     textinput.Model
	csvFocused   bool
	csvValid     bool
	defaultCSV   string

	// cron scheduler
	cronIdx      int           // index into cronLabels
	cronEnabled  bool
	nextRun      time.Time
	lastAutoRun  time.Time
	cronActive   bool          // ticker is ticking

	// layout
	width  int
	height int
}

func NewRunner(pythonPath, mainPath string) RunnerModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(Purple)

	vp := viewport.New(80, 20)

	ti := textinput.New()
	defaultCSV := "data/customer_feedback_raw.csv"
	ti.SetValue(defaultCSV)
	ti.Placeholder = "path/to/feedback.csv"
	ti.CharLimit = 200

	m := RunnerModel{
		state:      StateIdle,
		vp:         vp,
		sp:         sp,
		pythonPath: pythonPath,
		mainPath:   mainPath,
		parallel:   true,
		csvInput:   ti,
		defaultCSV: defaultCSV,
		cronIdx:    0,
	}
	m.csvValid = m.checkCSVExists()
	return m
}

func (m RunnerModel) WithSize(w, h int) RunnerModel {
	m.width = w
	m.height = h
	m.vp.Width = w - 4
	m.vp.Height = h - 18
	return m
}

func (m RunnerModel) checkCSVExists() bool {
	path := m.csvInput.Value()
	_, err := os.Stat(path)
	return err == nil
}

// ── Commands ──────────────────────────────────────────────────────────────────

func (m *RunnerModel) runPipeline() tea.Cmd {
	ch := m.outputCh
	pythonPath := m.pythonPath
	mainPath   := m.mainPath
	parallel   := m.parallel
	csvPath    := m.csvInput.Value()

	return func() tea.Msg {
		args := []string{"-u", mainPath}
		if parallel {
			args = append(args, "--parallel")
		}
		if csvPath != "" {
			args = append(args, "--input", csvPath)
		}

		cmd := exec.Command(pythonPath, args...)
		cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return PipelineDoneMsg{Err: err}
		}
		cmd.Stderr = cmd.Stdout

		if err := cmd.Start(); err != nil {
			return PipelineDoneMsg{Err: err}
		}

		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				ch <- scanner.Text()
			}
			cmd.Wait()
			close(ch)
		}()

		line, ok := <-ch
		if !ok {
			return PipelineDoneMsg{}
		}
		return PipelineLineMsg(line)
	}
}

func listenForOutput(ch chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return PipelineDoneMsg{}
		}
		return PipelineLineMsg(line)
	}
}

// cronTick fires a ScheduleTickMsg every second so we can update the countdown
func cronTick() tea.Cmd {
	return tea.Every(time.Second, func(t time.Time) tea.Msg {
		return ScheduleTickMsg(t)
	})
}

// ── Start / trigger helpers ───────────────────────────────────────────────────

func (m *RunnerModel) startRun(cmds *[]tea.Cmd) {
	if m.state == StateRunning {
		return
	}
	m.state    = StateRunning
	m.lines    = []string{}
	m.outputCh = make(chan string, 200)
	*cmds = append(*cmds, m.runPipeline(), m.sp.Tick)
}

func (m *RunnerModel) setCron(idx int) {
	m.cronIdx     = idx
	m.cronEnabled = idx != int(CronManual)
	m.cronActive  = m.cronEnabled
	if m.cronEnabled {
		m.nextRun = time.Now().Add(cronDurations[idx])
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m RunnerModel) Update(msg tea.Msg) (RunnerModel, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd  tea.Cmd

	switch msg := msg.(type) {

	// ── CSV input focused ─────────────────────────────────────────────────
	case tea.KeyMsg:
		if m.csvFocused {
			switch msg.String() {
			case "enter", "esc":
				m.csvFocused = false
				m.csvInput.Blur()
				m.csvValid = m.checkCSVExists()
			default:
				m.csvInput, cmd = m.csvInput.Update(msg)
				m.csvValid = m.checkCSVExists()
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		// ── Normal mode keys ──────────────────────────────────────────────
		switch msg.String() {
		case "enter", " ":
			if m.state != StateRunning {
				m.startRun(&cmds)
				// reset cron countdown after manual run
				if m.cronEnabled {
					m.nextRun = time.Now().Add(cronDurations[m.cronIdx])
				}
			}
		case "p":
			m.parallel = !m.parallel

		case "f":
			// focus CSV file input
			m.csvFocused = true
			m.csvInput.Focus()
			cmds = append(cmds, textinput.Blink)

		case "d":
			// reset to default CSV
			m.csvInput.SetValue(m.defaultCSV)
			m.csvValid = m.checkCSVExists()

		case "c":
			// cycle cron interval
			next := (m.cronIdx + 1) % len(cronLabels)
			m.setCron(next)
			if m.cronActive {
				cmds = append(cmds, cronTick())
			}

		case "x":
			// disable cron
			m.setCron(0)
		}

		m.vp, cmd = m.vp.Update(msg)
		cmds = append(cmds, cmd)

	// ── Pipeline output ───────────────────────────────────────────────────
	case PipelineLineMsg:
		m.lines = append(m.lines, string(msg))
		m.vp.SetContent(m.renderOutput())
		m.vp.GotoBottom()
		cmds = append(cmds, listenForOutput(m.outputCh))

	case PipelineDoneMsg:
		if msg.Err != nil {
			m.state = StateError
			m.lines = append(m.lines, "ERROR: "+msg.Err.Error())
		} else {
			m.state = StateDone
			m.lines = append(m.lines, "")
			m.lines = append(m.lines, "✓ Pipeline completed successfully!")
			m.lastAutoRun = time.Now()
		}
		m.vp.SetContent(m.renderOutput())
		m.vp.GotoBottom()
		// reschedule next cron run
		if m.cronEnabled {
			m.nextRun = time.Now().Add(cronDurations[m.cronIdx])
		}

	// ── Cron tick (every second) ──────────────────────────────────────────
	case ScheduleTickMsg:
		if m.cronEnabled && m.state != StateRunning {
			remaining := time.Until(m.nextRun)
			if remaining <= 0 {
				// Trigger auto-run
				m.lines = append(m.lines, fmt.Sprintf(
					"[cron] Auto-run triggered at %s", time.Now().Format("15:04:05"),
				))
				m.startRun(&cmds)
				m.nextRun = time.Now().Add(cronDurations[m.cronIdx])
			}
			// Keep ticking
			cmds = append(cmds, cronTick())
		}

	// ── Spinner ───────────────────────────────────────────────────────────
	case spinner.TickMsg:
		if m.state == StateRunning {
			m.sp, cmd = m.sp.Update(msg)
			cmds = append(cmds, cmd)
		}

	// ── External trigger (e.g. from dashboard) ────────────────────────────
	case CronFireMsg:
		if m.state != StateRunning {
			m.startRun(&cmds)
		}
	}

	return m, tea.Batch(cmds...)
}

// ── Render helpers ────────────────────────────────────────────────────────────

func (m RunnerModel) renderOutput() string {
	var sb strings.Builder
	for _, line := range m.lines {
		switch {
		case strings.Contains(line, "✓") || strings.Contains(line, "complete") || strings.Contains(line, "Done"):
			sb.WriteString(StyleStatusOK.Render(line) + "\n")
		case strings.Contains(line, "Error") || strings.Contains(line, "ERROR"):
			sb.WriteString(StyleStatusErr.Render(line) + "\n")
		case strings.Contains(line, "[Stage") || strings.Contains(line, "[cron]"):
			sb.WriteString(StyleAccent.Render(line) + "\n")
		case strings.Contains(line, "↳") || strings.Contains(line, "[clean]") || strings.Contains(line, "[runs]"):
			sb.WriteString(StyleMuted.Render(line) + "\n")
		default:
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

func (m RunnerModel) View() string {
	// ── Mode badge ─────────────────────────────────────────────────────────
	modeBadge := lipgloss.NewStyle().Foreground(White).Background(Gray).Padding(0, 1).Render("→ SEQUENTIAL")
	if m.parallel {
		modeBadge = lipgloss.NewStyle().Foreground(White).Background(Purple).Padding(0, 1).Render("⚡ PARALLEL")
	}

	// ── Status line ────────────────────────────────────────────────────────
	modeLabel := "sequential"
	if m.parallel { modeLabel = "parallel" }
	var statusLine string
	switch m.state {
	case StateIdle:
		statusLine = StyleMuted.Render(fmt.Sprintf("  Ready — press ENTER to run (%s)", modeLabel))
	case StateRunning:
		statusLine = m.sp.View() + StyleStatusRun.Render(fmt.Sprintf(" Running (%s)…  %d lines", modeLabel, len(m.lines)))
	case StateDone:
		statusLine = StyleStatusOK.Render(fmt.Sprintf("  ✓ Done (%s) — %d lines", modeLabel, len(m.lines)))
	case StateError:
		statusLine = StyleStatusErr.Render("  ✗ Failed — see output below")
	}

	// ── CSV file picker ────────────────────────────────────────────────────
	csvIcon := StyleStatusOK.Render("✓")
	csvBorder := lipgloss.Color("#4B5563")
	if !m.csvValid {
		csvIcon = StyleStatusErr.Render("✗")
		csvBorder = Red
	}
	csvStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(csvBorder).Padding(0, 1)
	csvLabel := StyleMuted.Render("  📂 CSV File: ") + csvIcon
	if m.csvFocused {
		csvLabel = StyleAccent.Render("  📂 CSV File: ") + csvIcon + StyleMuted.Render(" (editing…)")
	}
	csvBox := csvLabel + "\n" + csvStyle.Render(m.csvInput.View())
	if !m.csvValid && m.csvInput.Value() != "" {
		csvBox += "\n" + StyleStatusErr.Render("    File not found: "+m.csvInput.Value())
	}

	// ── Cron scheduler panel ───────────────────────────────────────────────
	cronLabel := cronLabels[m.cronIdx]
	var cronBadge string
	if m.cronEnabled {
		cronBadge = lipgloss.NewStyle().Foreground(White).Background(Green).Padding(0, 1).Render("ACTIVE")
	} else {
		cronBadge = lipgloss.NewStyle().Foreground(White).Background(Gray).Padding(0, 1).Render("OFF")
	}

	cronCountdown := ""
	if m.cronEnabled {
		remaining := time.Until(m.nextRun)
		if remaining < 0 { remaining = 0 }
		h  := int(remaining.Hours())
		mi := int(remaining.Minutes()) % 60
		s  := int(remaining.Seconds()) % 60
		var countdown string
		if h > 0 {
			countdown = fmt.Sprintf("%dh %02dm %02ds", h, mi, s)
		} else if mi > 0 {
			countdown = fmt.Sprintf("%dm %02ds", mi, s)
		} else {
			countdown = fmt.Sprintf("%ds", s)
		}
		cronCountdown = "  Next run in: " + StyleBold.Render(countdown)
		if !m.lastAutoRun.IsZero() {
			cronCountdown += StyleMuted.Render("  (last: "+m.lastAutoRun.Format("15:04:05")+")")
		}
	}

	cronBar := fmt.Sprintf("  ⏰ Schedule: %s  Interval: %s%s",
		cronBadge,
		StyleBold.Render(cronLabel),
		func() string {
			if cronCountdown != "" { return "\n" + cronCountdown }
			return ""
		}(),
	)
	cronCard := StyleCard.Width(m.width - 4).Render(cronBar)

	// ── Output viewport ────────────────────────────────────────────────────
	outputBox := StyleCard.Width(m.width - 4).Render(m.vp.View())

	help := StyleHelp.Render(
		"  [enter] run  [p] parallel  [f] file input  [d] default CSV  [c] cycle schedule  [x] disable  [↑↓] scroll",
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		"  "+modeBadge+"  "+statusLine,
		"",
		csvBox,
		"",
		cronCard,
		"",
		outputBox,
		"",
		help,
	)
}
