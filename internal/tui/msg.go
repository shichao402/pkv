package tui

import (
	"github.com/shichao402/pkv/internal/app"
	bwtypes "github.com/shichao402/pkv/internal/bw/types"
)

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

type foldersLoadedMsg struct {
	folders []bwtypes.Folder
	err     error
}

type resourcesLoadedMsg struct {
	resources app.BrowseResources
	err       error
}

type operationResultMsg struct {
	message string
	reload  bool
	err     error
}
