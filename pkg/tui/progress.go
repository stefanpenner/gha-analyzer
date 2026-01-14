package tui

import (
	"fmt"
	"io"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type Progress struct {
	program *tea.Program
	done    chan struct{}
	once    sync.Once
}

type startURLMsg struct {
	index int
	total int
	url   string
}

type setURLRunsMsg struct {
	count int
}

type setPhaseMsg struct {
	phase string
}

type setDetailMsg struct {
	detail string
}

type processRunMsg struct{}

type finishMsg struct{}

type progressModel struct {
	totalURLs      int
	currentURL     int
	currentRun     int
	currentURLRuns int
	currentURLText string
	phase          string
	detail         string
	done           bool
}

func NewProgress(totalURLs int, output io.Writer) *Progress {
	model := progressModel{totalURLs: totalURLs}
	program := tea.NewProgram(model, tea.WithOutput(output))
	return &Progress{
		program: program,
		done:    make(chan struct{}),
	}
}

func (p *Progress) Start() {
	go func() {
		defer close(p.done)
		_, _ = p.program.Run()
	}()
}

func (p *Progress) Wait() {
	<-p.done
}

func (p *Progress) StartURL(index int, url string) {
	p.program.Send(startURLMsg{index: index, total: 0, url: url})
}

func (p *Progress) SetURLRuns(runCount int) {
	p.program.Send(setURLRunsMsg{count: runCount})
}

func (p *Progress) SetPhase(phase string) {
	p.program.Send(setPhaseMsg{phase: phase})
}

func (p *Progress) SetDetail(detail string) {
	p.program.Send(setDetailMsg{detail: detail})
}

func (p *Progress) ProcessRun() {
	p.program.Send(processRunMsg{})
}

func (p *Progress) Finish() {
	p.program.Send(finishMsg{})
	p.once.Do(func() {
		p.program.Quit()
	})
}

func (m progressModel) Init() tea.Cmd {
	return nil
}

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case startURLMsg:
		m.currentURL = typed.index + 1
		m.currentRun = 0
		m.currentURLText = typed.url
		return m, nil
	case setURLRunsMsg:
		m.currentURLRuns = typed.count
		return m, nil
	case setPhaseMsg:
		m.phase = typed.phase
		return m, nil
	case setDetailMsg:
		m.detail = typed.detail
		return m, nil
	case processRunMsg:
		m.currentRun++
		return m, nil
	case finishMsg:
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m progressModel) View() string {
	if m.done {
		return ""
	}
	urlProgress := fmt.Sprintf("%d/%d", m.currentURL, m.totalURLs)
	runProgress := ""
	if m.currentURLRuns > 0 {
		runProgress = fmt.Sprintf(" (%d/%d runs)", m.currentRun, m.currentURLRuns)
	}
	parts := []string{fmt.Sprintf("Processing URL %s%s", urlProgress, runProgress)}
	if m.currentURLText != "" {
		parts = append(parts, m.currentURLText)
	}
	if m.phase != "" {
		parts = append(parts, fmt.Sprintf("Phase: %s", m.phase))
	}
	if m.detail != "" {
		parts = append(parts, fmt.Sprintf("Detail: %s", m.detail))
	}
	return strings.Join(parts, " • ")
}
