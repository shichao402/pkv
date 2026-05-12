package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/shichao402/pkv/internal/app"
	bwtypes "github.com/shichao402/pkv/internal/bw/types"
)

type focusMode int

const (
	focusFolders focusMode = iota
	focusResources
	focusDetail
)

type resourceTab int

const (
	tabSSH resourceTab = iota
	tabEnv
	tabNotes
)

type keyMap struct {
	up     key.Binding
	down   key.Binding
	left   key.Binding
	right  key.Binding
	tab    key.Binding
	enter  key.Binding
	escape key.Binding
	reload key.Binding
	quit   key.Binding
	ctrlC  key.Binding
}

type Model struct {
	ctx      context.Context
	reporter *Reporter
	keys     keyMap

	folders        []bwtypes.Folder
	selectedFolder int
	currentFolder  *bwtypes.Folder
	resources      app.BrowseResources
	selectedItem   map[resourceTab]int
	tab            resourceTab
	focus          focusMode

	loading bool
	status  string
	err     error
	width   int
	height  int
}

func NewModel(ctx context.Context) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	return Model{
		ctx:          ctx,
		reporter:     NewReporter(),
		keys:         defaultKeyMap(),
		selectedItem: map[resourceTab]int{tabSSH: 0, tabEnv: 0, tabNotes: 0},
		status:       "Loading folders...",
		loading:      true,
	}
}

func Run(ctx context.Context) error {
	_, err := tea.NewProgram(NewModel(ctx), tea.WithAltScreen()).Run()
	if err != nil {
		return fmt.Errorf("tui failed: %w", err)
	}
	return nil
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadFoldersCmd(), m.reporter.waitStatus())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case statusMsg:
		m.status = msg.message
		if msg.level == statusError {
			m.err = fmt.Errorf("%s", msg.message)
		}
		return m, m.reporter.waitStatus()
	case foldersLoadedMsg:
		m.loading = false
		m.folders = msg.folders
		m.err = msg.err
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		if len(m.folders) == 0 {
			m.status = "No folders found."
			return m, nil
		}
		m.status = fmt.Sprintf("Loaded %d folder(s).", len(m.folders))
		folder := m.folders[m.selectedFolder]
		m.currentFolder = &folder
		m.loading = true
		m.status = fmt.Sprintf("Loading %s...", folder.Name)
		return m, m.loadResourcesCmd(folder)
	case resourcesLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.resources = msg.resources
		m.status = fmt.Sprintf("Loaded %s: %d SSH, env %s, %d note(s).", m.resources.Folder.Name, len(m.resources.SSHKeys), yesNo(m.resources.EnvNote != nil), len(m.resources.Notes))
		m.clampSelection()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
		return m, nil
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.quit), key.Matches(msg, m.keys.ctrlC):
		return m, tea.Quit
	case key.Matches(msg, m.keys.escape):
		if m.focus == focusDetail {
			m.focus = focusResources
		} else if m.focus == focusResources {
			m.focus = focusFolders
		}
		return m, nil
	case key.Matches(msg, m.keys.reload):
		m.err = nil
		m.loading = true
		if m.focus == focusFolders || m.currentFolder == nil {
			m.status = "Reloading folders..."
			return m, m.loadFoldersCmd()
		}
		m.status = fmt.Sprintf("Reloading %s...", m.currentFolder.Name)
		return m, m.loadResourcesCmd(*m.currentFolder)
	case key.Matches(msg, m.keys.up):
		m.moveSelection(-1)
		return m, nil
	case key.Matches(msg, m.keys.down):
		m.moveSelection(1)
		return m, nil
	case key.Matches(msg, m.keys.left):
		if m.focus == focusResources {
			m.previousTab()
		} else if m.focus == focusDetail {
			m.focus = focusResources
		}
		return m, nil
	case key.Matches(msg, m.keys.right), key.Matches(msg, m.keys.tab):
		if m.focus == focusFolders && m.currentFolder != nil {
			m.focus = focusResources
		} else if m.focus == focusResources {
			m.nextTab()
		}
		return m, nil
	case key.Matches(msg, m.keys.enter):
		return m.handleEnter()
	default:
		return m, nil
	}
}

func (m Model) View() string {
	return render(m)
}

func (m Model) loadFoldersCmd() tea.Cmd {
	return func() tea.Msg {
		folders, err := app.BrowseFolders(m.ctx, m.reporter)
		return foldersLoadedMsg{folders: folders, err: err}
	}
}

func (m Model) loadResourcesCmd(folder bwtypes.Folder) tea.Cmd {
	return func() tea.Msg {
		resources, err := app.BrowseFolderResources(m.ctx, folder, m.reporter)
		return resourcesLoadedMsg{resources: resources, err: err}
	}
}

func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.focus {
	case focusFolders:
		if len(m.folders) == 0 {
			return *m, nil
		}
		folder := m.folders[m.selectedFolder]
		m.currentFolder = &folder
		m.focus = focusResources
		m.loading = true
		m.err = nil
		m.status = fmt.Sprintf("Loading %s...", folder.Name)
		return *m, m.loadResourcesCmd(folder)
	case focusResources:
		if len(m.currentItems()) > 0 || m.tab == tabEnv && m.resources.EnvNote != nil {
			m.focus = focusDetail
		}
	}
	return *m, nil
}

func (m *Model) moveSelection(delta int) {
	if m.focus == focusFolders {
		m.selectedFolder = clamp(m.selectedFolder+delta, len(m.folders))
		return
	}
	m.selectedItem[m.tab] = clamp(m.selectedItem[m.tab]+delta, len(m.currentItems()))
}

func (m *Model) nextTab() {
	m.tab = (m.tab + 1) % 3
	m.clampSelection()
}

func (m *Model) previousTab() {
	if m.tab == tabSSH {
		m.tab = tabNotes
	} else {
		m.tab--
	}
	m.clampSelection()
}

func (m *Model) currentItems() []bwtypes.Item {
	switch m.tab {
	case tabSSH:
		return m.resources.SSHKeys
	case tabEnv:
		if m.resources.EnvNote == nil {
			return nil
		}
		return []bwtypes.Item{*m.resources.EnvNote}
	case tabNotes:
		return m.resources.Notes
	default:
		return nil
	}
}

func (m *Model) currentItem() (bwtypes.Item, bool) {
	items := m.currentItems()
	if len(items) == 0 {
		return bwtypes.Item{}, false
	}
	idx := clamp(m.selectedItem[m.tab], len(items))
	return items[idx], true
}

func (m *Model) clampSelection() {
	m.selectedFolder = clamp(m.selectedFolder, len(m.folders))
	m.selectedItem[m.tab] = clamp(m.selectedItem[m.tab], len(m.currentItems()))
}

func defaultKeyMap() keyMap {
	return keyMap{
		up:     key.NewBinding(key.WithKeys("up", "k")),
		down:   key.NewBinding(key.WithKeys("down", "j")),
		left:   key.NewBinding(key.WithKeys("left", "h")),
		right:  key.NewBinding(key.WithKeys("right", "l")),
		tab:    key.NewBinding(key.WithKeys("tab")),
		enter:  key.NewBinding(key.WithKeys("enter")),
		escape: key.NewBinding(key.WithKeys("esc")),
		reload: key.NewBinding(key.WithKeys("r")),
		quit:   key.NewBinding(key.WithKeys("q")),
		ctrlC:  key.NewBinding(key.WithKeys("ctrl+c")),
	}
}

func clamp(value, size int) int {
	if size <= 0 || value < 0 {
		return 0
	}
	if value >= size {
		return size - 1
	}
	return value
}

func yesNo(ok bool) string {
	if ok {
		return "yes"
	}
	return "none"
}
