package tui

import "github.com/shichao402/pkv/internal/app"

type statusLevel int

const (
	statusInfo statusLevel = iota
	statusWarn
	statusError
)

type statusMsg struct {
	level   statusLevel
	message string
}

type vaultLoadedMsg struct {
	requestID uint64
	snapshot  app.BrowseSnapshot
	err       error
}

type operationResultMsg struct {
	message string
	reload  bool
	err     error
}
