package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type event struct {
	message   string
	added     int
	retry     time.Duration
	mainRetry time.Duration
}

type Reporter struct {
	program       *tea.Program
	events        chan event
	done          chan struct{}
	interrupt     chan struct{}
	closeOnce     sync.Once
	interruptOnce sync.Once
}

// Start launches the terminal UI for an operation with a known record count.
func Start(total int) *Reporter {
	r := &Reporter{
		events:    make(chan event, 256),
		done:      make(chan struct{}),
		interrupt: make(chan struct{}),
	}
	m := model{
		total:      total,
		logs:       []string{"Operation started"},
		started:    time.Now(),
		events:     r.events,
		autoScroll: true,
		interrupt:  r.requestInterrupt,
	}
	r.program = tea.NewProgram(m, tea.WithAltScreen())
	go func() {
		_, _ = r.program.Run()
		close(r.done)
	}()
	return r
}

func (r *Reporter) send(e event) {
	if r == nil {
		return
	}
	select {
	case r.events <- e:
	default:
	}
}

func (r *Reporter) Log(message string) { r.send(event{message: message}) }

func (r *Reporter) Success(count int, message string) {
	r.send(event{message: message, added: count})
}

func (r *Reporter) Failure(message string) { r.send(event{message: message}) }

func (r *Reporter) SetRetryTimer(delay time.Duration) { r.send(event{retry: delay}) }

func (r *Reporter) SetMainRetryTimer(delay time.Duration) {
	r.send(event{mainRetry: delay, retry: delay})
}

// Interrupt is closed when the user presses CTRL-C in the TUI.
func (r *Reporter) Interrupt() <-chan struct{} {
	if r == nil {
		return nil
	}
	return r.interrupt
}

func (r *Reporter) requestInterrupt() {
	r.interruptOnce.Do(func() { close(r.interrupt) })
}

// Close returns control of the terminal to the caller.
func (r *Reporter) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		r.program.Quit()
		<-r.done
	})
}

type tickMsg time.Time
type eventMsg event

type model struct {
	total      int
	completed  int
	retry      time.Duration
	mainRetry  time.Duration
	started    time.Time
	logs       []string
	scroll     int
	width      int
	height     int
	autoScroll bool
	events     <-chan event
	interrupt  func()
}

func (m model) Init() tea.Cmd {
	return tea.Batch(waitForEvent(m.events), tick())
}

func waitForEvent(events <-chan event) tea.Cmd {
	return func() tea.Msg { return eventMsg(<-events) }
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case eventMsg:
		if msg.message != "" {
			m.logs = append(m.logs, msg.message)
		}
		m.completed += msg.added
		if msg.retry > 0 {
			m.retry = msg.retry
		}
		if msg.mainRetry > 0 {
			m.mainRetry = msg.mainRetry
		}
		if m.autoScroll {
			m.scroll = m.maxScroll()
		}
		return m, waitForEvent(m.events)
	case tickMsg:
		return m, tick()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.interrupt != nil {
				m.interrupt()
			}
			return m, tea.Quit
		case "up", "k":
			m.autoScroll = false
			m.scroll--
		case "down", "j":
			m.scroll++
		case "pgup":
			m.autoScroll = false
			m.scroll -= max(1, m.height/3)
		case "pgdown":
			m.scroll += max(1, m.height/3)
		case "home":
			m.autoScroll = false
			m.scroll = 0
		case "end":
			m.scroll = m.maxScroll()
		}
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
	if m.scroll > m.maxScroll() {
		m.scroll = m.maxScroll()
	}
	if m.scroll == m.maxScroll() {
		m.autoScroll = true
	}
	return m, nil
}

func (m model) logHeight() int { return max(4, m.height-12) }

func (m model) maxScroll() int { return max(0, len(m.logs)-m.logHeight()) }

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Starting pxctl..."
	}
	innerWidth := max(20, m.width-4)
	logHeight := m.logHeight()

	logs := m.logs
	start := min(m.scroll, len(logs))
	end := min(len(logs), start+logHeight)
	logLines := make([]string, 0, logHeight)
	for _, line := range logs[start:end] {
		logLines = append(logLines, renderLogLine(line, innerWidth))
	}
	for len(logLines) < logHeight {
		logLines = append(logLines, "")
	}

	frame := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(innerWidth)
	return lipgloss.JoinVertical(lipgloss.Left,
		frame.Render(statusView(m, innerWidth)),
		frame.Render(strings.Join(logLines, "\n")),
	)
}

func statusView(m model, width int) string {
	elapsed := time.Since(m.started).Seconds()
	throughput := 0.0
	if elapsed > 0 {
		throughput = float64(m.completed) / elapsed
	}
	remaining := max(0, m.total-m.completed)
	finish := "calculating"
	if throughput > 0 {
		finish = time.Now().Add(time.Duration(float64(remaining)/throughput) * time.Second).Format("15:04:05")
	}
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	value := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	return lipgloss.NewStyle().Width(width).Render(strings.Join([]string{
		label.Render("STATUS"),
		fmt.Sprintf("%s %s", label.Render("Records inserted:"), value.Render(fmt.Sprintf("%d/%d", m.completed, m.total))),
		fmt.Sprintf("%s %s", label.Render("Records to go:"), value.Render(fmt.Sprintf("%d", remaining))),
		fmt.Sprintf("%s %s", label.Render("Throughput:"), value.Render(fmt.Sprintf("%.1f records/sec", throughput))),
		fmt.Sprintf("%s %s", label.Render("Main retry timer:"), warn.Render(retryString(m.mainRetry))),
		fmt.Sprintf("%s %s", label.Render("Active retry:"), warn.Render(retryString(m.retry))),
		fmt.Sprintf("%s %s", label.Render("Estimated finish:"), value.Render(finish)),
	}, "\n"))
}

func renderLogLine(line string, width int) string {
	lower := strings.ToLower(line)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	glyph := "·"
	switch {
	case strings.Contains(lower, "429") || strings.Contains(lower, "backing off"):
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		glyph = "⚠"
	case strings.Contains(lower, "failed") || strings.Contains(lower, "error") || strings.Contains(lower, "skipped"):
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		glyph = "✗"
	case strings.Contains(lower, "succeeded") || strings.Contains(lower, "successfully") || strings.Contains(lower, "completed"):
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
		glyph = "✓"
	case strings.Contains(lower, "submitting") || strings.Contains(lower, "started"):
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
		glyph = "→"
	}
	return style.Render(truncate(glyph+" "+line, width))
}

func retryString(delay time.Duration) string {
	if delay <= 0 {
		return "not set"
	}
	return delay.Round(time.Millisecond).String()
}

func truncate(value string, width int) string {
	if len(value) <= width {
		return value
	}
	return value[:max(0, width-3)] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
