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

type reloadKind int

const (
	reloadNone reloadKind = iota
	reloadFolder
	reloadVault
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

type folderLoadedMsg struct {
	requestID uint64
	folderID  string
	resources app.BrowseResources
	err       error
}

type operationResultMsg struct {
	message    string
	session    string
	reloadKind reloadKind
	folderID   string
	err        error
}

type editorFinishedMsg struct {
	state   editState
	content string
	err     error
}

func folderParams(folder bwtypes.Folder) (name, id string) {
	if folder.ID == "" {
		return folder.Name, ""
	}
	return folder.Name, folder.ID
}
