package app

import (
	"fmt"
	"io"
)

// Reporter receives user-visible events from app abilities.
// CLI and TUI frontends adapt these events to their own presentation layer.
type Reporter interface {
	Info(message string)
	Infof(format string, args ...any)
	Warn(message string)
	Warnf(format string, args ...any)
	Error(message string)
	Errorf(format string, args ...any)
}

type noopReporter struct{}

func (noopReporter) Info(string)           {}
func (noopReporter) Infof(string, ...any)  {}
func (noopReporter) Warn(string)           {}
func (noopReporter) Warnf(string, ...any)  {}
func (noopReporter) Error(string)          {}
func (noopReporter) Errorf(string, ...any) {}

func reporterOrNoop(r Reporter) Reporter {
	if r == nil {
		return noopReporter{}
	}
	return r
}

// TextReporter writes informational events to Out and warnings/errors to Err.
type TextReporter struct {
	Out io.Writer
	Err io.Writer
}

func (r TextReporter) Info(message string) {
	if r.Out == nil {
		return
	}
	_, _ = fmt.Fprintln(r.Out, message)
}

func (r TextReporter) Infof(format string, args ...any) {
	if r.Out == nil {
		return
	}
	_, _ = fmt.Fprintf(r.Out, format, args...)
}

func (r TextReporter) Warn(message string) {
	if r.Err == nil {
		return
	}
	_, _ = fmt.Fprintln(r.Err, message)
}

func (r TextReporter) Warnf(format string, args ...any) {
	if r.Err == nil {
		return
	}
	_, _ = fmt.Fprintf(r.Err, format, args...)
}

func (r TextReporter) Error(message string) {
	if r.Err == nil {
		return
	}
	_, _ = fmt.Fprintln(r.Err, message)
}

func (r TextReporter) Errorf(format string, args ...any) {
	if r.Err == nil {
		return
	}
	_, _ = fmt.Fprintf(r.Err, format, args...)
}
