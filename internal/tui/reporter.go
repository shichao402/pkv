package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Reporter struct {
	messages chan tea.Msg
}

func NewReporter() *Reporter {
	return &Reporter{messages: make(chan tea.Msg, 16)}
}

func (r *Reporter) Info(message string) {
	r.send(statusMsg{level: statusInfo, message: strings.TrimSpace(message)})
}

func (r *Reporter) Infof(format string, args ...any) {
	r.Info(fmt.Sprintf(format, args...))
}

func (r *Reporter) Warn(message string) {
	r.send(statusMsg{level: statusWarn, message: strings.TrimSpace(message)})
}

func (r *Reporter) Warnf(format string, args ...any) {
	r.Warn(fmt.Sprintf(format, args...))
}

func (r *Reporter) Error(message string) {
	r.send(statusMsg{level: statusError, message: strings.TrimSpace(message)})
}

func (r *Reporter) Errorf(format string, args ...any) {
	r.Error(fmt.Sprintf(format, args...))
}

func (r *Reporter) waitStatus() tea.Cmd {
	return func() tea.Msg {
		return <-r.messages
	}
}

func (r *Reporter) send(msg tea.Msg) {
	if r == nil || r.messages == nil {
		return
	}
	select {
	case r.messages <- msg:
	default:
	}
}
