package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type PipelineState int

const (
	StateIdle PipelineState = iota
	StateRunning
	StateDone
	StateError
)

// Messages for streaming output
type PipelineLineMsg string
type PipelineDoneMsg struct{ Err error }

// RunnerModel handles running the Python pipeline with live output
type RunnerModel struct {
	state      PipelineState
	vp         viewport.Model
	spinner    spinner.Model
	lines      []string
	pythonPath string
	mainPath   string
	parallel   bool
	width      int
	height     int
	outputCh   chan string
}

func NewRunner(pythonPath, mainPath string) RunnerModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(Purple)

	vp := viewport.New(80, 20)

	return RunnerModel{
		state:      StateIdle,
		vp:         vp,
		spinner:    sp,
		pythonPath: pythonPath,
		mainPath:   mainPath,
		parallel:   true,
	}
}

func (m RunnerModel) WithSize(w, h int) RunnerModel {
	m.width = w
	m.height = h
	m.vp.Width = w - 4
	m.vp.Height = h - 14
	return m
}

// startPipeline kicks off the pipeline subprocess and streams output
func (m *RunnerModel) runPipelineStreaming() tea.Cmd {
	ch := m.outputCh
	pythonPath := m.pythonPath
	mainPath := m.mainPath
	parallel := m.parallel

	return func() tea.Msg {
		args := []string{"-u", mainPath} // -u = unbuffered stdout
		if parallel {
			args = append(args, "--parallel")
		}

		cmd := exec.Command(pythonPath, args...)

		// Inherit full OS environment so Python can find venv packages,
		// then override/add PYTHONUNBUFFERED so output streams in real-time
		cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return PipelineDoneMsg{Err: err}
		}
		cmd.Stderr = cmd.Stdout // merge stderr into same pipe

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

		// Wait for first line (or channel close)
		line, ok := <-ch
		if !ok {
			return PipelineDoneMsg{}
		}
		return PipelineLineMsg(line)
	}
}

// listenForOutput waits for next line from channel
func listenForOutput(ch chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return PipelineDoneMsg{}
		}
		return PipelineLineMsg(line)
	}
}

func (m RunnerModel) Update(msg tea.Msg) (RunnerModel, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ":
			if m.state == StateIdle || m.state == StateDone || m.state == StateError {
				m.state = StateRunning
				m.lines = []string{}
				m.outputCh = make(chan string, 100)
				cmds = append(cmds, m.runPipelineStreaming(), m.spinner.Tick)
			}
		case "p":
			m.parallel = !m.parallel
		}
		m.vp, cmd = m.vp.Update(msg)
		cmds = append(cmds, cmd)

	case PipelineLineMsg:
		line := string(msg)
		m.lines = append(m.lines, line)
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
		}
		m.vp.SetContent(m.renderOutput())
		m.vp.GotoBottom()

	case spinner.TickMsg:
		if m.state == StateRunning {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}




func (m RunnerModel) renderOutput() string {
	var sb strings.Builder
	for _, line := range m.lines {
		// Color-code key lines
		switch {
		case strings.Contains(line, "✓") || strings.Contains(line, "complete") || strings.Contains(line, "Done"):
			sb.WriteString(StyleStatusOK.Render(line) + "\n")
		case strings.Contains(line, "Error") || strings.Contains(line, "ERROR"):
			sb.WriteString(StyleStatusErr.Render(line) + "\n")
		case strings.Contains(line, "[Stage"):
			sb.WriteString(StyleAccent.Render(line) + "\n")
		case strings.Contains(line, "↳"):
			sb.WriteString(StyleMuted.Render(line) + "\n")
		default:
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

func (m RunnerModel) View() string {
	// Status header
	var statusLine string
	modeLabel := "sequential"
	if m.parallel {
		modeLabel = "parallel"
	}
	switch m.state {
	case StateIdle:
		statusLine = StyleMuted.Render(fmt.Sprintf("  Mode: [%s]  Press ENTER to run pipeline", modeLabel))
	case StateRunning:
		statusLine = m.spinner.View() + StyleStatusRun.Render(fmt.Sprintf(" Running (%s)…  %d lines captured", modeLabel, len(m.lines)))
	case StateDone:
		statusLine = StyleStatusOK.Render(fmt.Sprintf("  ✓ Completed (%s) — %d lines", modeLabel, len(m.lines)))
	case StateError:
		statusLine = StyleStatusErr.Render("  ✗ Pipeline failed — see output below")
	}

	// Output viewport box
	outputBox := StyleCard.Width(m.width - 4).Render(m.vp.View())

	help := StyleHelp.Render("  [enter] run  [p] toggle parallel/sequential  [↑↓] scroll output")

	parallelBadge := ""
	if m.parallel {
		parallelBadge = lipgloss.NewStyle().
			Foreground(White).Background(Purple).Padding(0, 1).
			Render("⚡ PARALLEL")
	} else {
		parallelBadge = lipgloss.NewStyle().
			Foreground(White).Background(lipgloss.Color("#374151")).Padding(0, 1).
			Render("→ SEQUENTIAL")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		"  "+parallelBadge+"  "+statusLine,
		"",
		outputBox,
		"",
		help,
	)
}
