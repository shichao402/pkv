package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/shichao402/pkv/internal/app"
	"github.com/shichao402/pkv/internal/bw"
	bwtypes "github.com/shichao402/pkv/internal/bw/types"
	pkvkey "github.com/shichao402/pkv/internal/key"
	"github.com/shichao402/pkv/internal/pathutil"
	"github.com/shichao402/pkv/internal/securenote"
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

type interactionMode int

const (
	interactionNone interactionMode = iota
	interactionConfirm
	interactionEdit
	interactionSSHWizard
)

type confirmKind int

const (
	confirmRemove confirmKind = iota
	confirmClean
)

type sshWizardStep int

const (
	sshStepPrivatePath sshWizardStep = iota
	sshStepPublicKey
	sshStepKeyName
	sshStepConfirm
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
	add    key.Binding
	edit   key.Binding
	delete key.Binding
	clean  key.Binding
	unlock key.Binding
	save   key.Binding
	quit   key.Binding
	ctrlC  key.Binding
}

type confirmState struct {
	kind confirmKind
	tab  resourceTab
	item bwtypes.Item
}

type editState struct {
	tab     resourceTab
	item    bwtypes.Item
	content textBuffer
}

type sshWizardState struct {
	step         sshWizardStep
	privateInput textBuffer
	publicInput  textBuffer
	nameInput    textBuffer
	openSSHKey   string
	derivedPub   string
	fingerprint  string
	err          string
}

type Model struct {
	ctx      context.Context
	reporter *Reporter
	keys     keyMap

	folders             []bwtypes.Folder
	selectedFolder      int
	currentFolder       *bwtypes.Folder
	resources           app.BrowseResources
	resourcesByFolderID map[string]app.BrowseResources
	selectedItem        map[resourceTab]int
	tab                 resourceTab
	focus               focusMode

	interaction interactionMode
	confirm     confirmState
	edit        editState
	sshWizard   sshWizardState

	loading bool
	status  string
	err     error
	width   int
	height  int

	loadSeq      uint64
	activeLoadID uint64
}

func NewModel(ctx context.Context) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	return Model{
		ctx:                 ctx,
		reporter:            NewReporter(),
		keys:                defaultKeyMap(),
		selectedItem:        map[resourceTab]int{tabSSH: 0, tabEnv: 0, tabNotes: 0},
		resourcesByFolderID: map[string]app.BrowseResources{},
		status:              "Loading vault...",
		loading:             true,
		loadSeq:             1,
		activeLoadID:        1,
	}
}

func Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := runInteractiveUnlock(ctx, app.TextReporter{Out: os.Stderr, Err: os.Stderr}); err != nil {
		return fmt.Errorf("tui authentication failed: %w", err)
	}

	_, err := tea.NewProgram(NewModel(ctx), tea.WithAltScreen()).Run()
	if err != nil {
		return fmt.Errorf("tui failed: %w", err)
	}
	return nil
}

func runInteractiveUnlock(ctx context.Context, reporter app.Reporter) error {
	_, _ = fmt.Fprintln(os.Stderr, "Checking Bitwarden unlock state. If bw asks for your master password, input is hidden; type it and press Enter.")
	_, err := app.Unlock(ctx, app.UnlockParams{}, reporter)
	return err
}

type unlockExecCommand struct {
	ctx      context.Context
	reporter app.Reporter
}

func (c unlockExecCommand) Run() error {
	return runInteractiveUnlock(c.ctx, c.reporter)
}

func (unlockExecCommand) SetStdin(io.Reader)  {}
func (unlockExecCommand) SetStdout(io.Writer) {}
func (unlockExecCommand) SetStderr(io.Writer) {}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadVaultCmd(m.activeLoadID), m.reporter.waitStatus())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeInputs()
		return m, nil
	case statusMsg:
		m.status = msg.message
		if msg.level == statusError {
			m.err = fmt.Errorf("%s", msg.message)
		}
		return m, m.reporter.waitStatus()
	case vaultLoadedMsg:
		if msg.requestID != m.activeLoadID {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}

		previousFolderID := ""
		if m.currentFolder != nil {
			previousFolderID = m.currentFolder.ID
		}
		m.folders = msg.snapshot.Folders
		m.resourcesByFolderID = msg.snapshot.ResourcesByFolderID
		if m.resourcesByFolderID == nil {
			m.resourcesByFolderID = map[string]app.BrowseResources{}
		}
		if len(m.folders) == 0 {
			m.currentFolder = nil
			m.resources = app.BrowseResources{}
			m.status = "No folders found."
			return m, nil
		}

		m.selectedFolder = selectFolderIndex(m.folders, previousFolderID, m.selectedFolder)
		m.applySelectedFolderFromCache()
		m.status = fmt.Sprintf("Loaded %d folder(s), %d item(s). Selected %s: %d SSH, env %s, %d note(s).", len(m.folders), msg.snapshot.ItemCount, m.resources.Folder.Name, len(m.resources.SSHKeys), yesNo(m.resources.EnvNote != nil), len(m.resources.Notes))
		return m, nil
	case operationResultMsg:
		m.loading = false
		m.err = msg.err
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.status = msg.message
		m.interaction = interactionNone
		if msg.reload {
			m.loading = true
			m.status = msg.message + " Refreshing vault..."
			requestID := m.beginLoad()
			return m, m.loadVaultCmd(requestID)
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
		return m, nil
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.interaction == interactionConfirm {
		return m.handleConfirmKey(msg)
	}
	if m.interaction == interactionEdit {
		return m.handleEditKey(msg)
	}
	if m.interaction == interactionSSHWizard {
		return m.handleSSHWizardKey(msg)
	}

	switch {
	case key.Matches(msg, m.keys.quit), key.Matches(msg, m.keys.ctrlC):
		return m, tea.Quit
	case key.Matches(msg, m.keys.escape):
		switch m.focus {
		case focusDetail:
			m.focus = focusResources
		case focusResources:
			m.focus = focusFolders
		}
		return m, nil
	case key.Matches(msg, m.keys.unlock):
		m.invalidateLoads()
		m.loading = true
		m.err = nil
		m.status = "Authenticating with Bitwarden..."
		return m, m.unlockCmd()
	case key.Matches(msg, m.keys.reload):
		m.err = nil
		m.loading = true
		m.status = "Refreshing vault..."
		requestID := m.beginLoad()
		return m, m.loadVaultCmd(requestID)
	case key.Matches(msg, m.keys.add):
		return m.startAdd()
	case key.Matches(msg, m.keys.edit):
		return m.startEdit()
	case key.Matches(msg, m.keys.delete):
		return m.startRemoveConfirm()
	case key.Matches(msg, m.keys.clean):
		return m.startCleanConfirm()
	case key.Matches(msg, m.keys.up):
		m.moveSelection(-1)
		return m, nil
	case key.Matches(msg, m.keys.down):
		m.moveSelection(1)
		return m, nil
	case key.Matches(msg, m.keys.left):
		switch m.focus {
		case focusResources:
			m.previousTab()
		case focusDetail:
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

func (m Model) loadVaultCmd(requestID uint64) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := app.BrowseVaultSnapshot(m.ctx, m.reporter)
		return vaultLoadedMsg{requestID: requestID, snapshot: snapshot, err: err}
	}
}

func (m Model) unlockCmd() tea.Cmd {
	return tea.Exec(unlockExecCommand{ctx: m.ctx, reporter: m.reporter}, func(err error) tea.Msg {
		if err != nil {
			return operationResultMsg{err: err}
		}
		return operationResultMsg{message: "Vault unlocked.", reload: true}
	})
}

func (m Model) removeCmd(state confirmState) tea.Cmd {
	folder := m.folderName()
	kind := tabKind(state.tab)
	ids := idsForAction(state.tab, state.item)
	return func() tea.Msg {
		result, err := app.Remove(m.ctx, app.RemoveParams{Folder: folder, Kind: kind, IDs: ids}, m.reporter)
		if err != nil {
			return operationResultMsg{err: err}
		}
		return operationResultMsg{message: fmt.Sprintf("Removed %d %s item(s).", result.Removed, kind), reload: true}
	}
}

func (m Model) cleanCmd(state confirmState) tea.Cmd {
	folder := m.folderName()
	kind := tabKind(state.tab)
	return func() tea.Msg {
		result, err := app.Clean(m.ctx, app.CleanParams{Folder: folder, Kind: kind}, m.reporter)
		if err != nil {
			return operationResultMsg{err: err}
		}
		return operationResultMsg{message: fmt.Sprintf("Cleaned %d %s item(s).", result.Cleaned, kind), reload: true}
	}
}

func (m Model) saveEditCmd(state editState) tea.Cmd {
	folder := m.folderName()
	content := state.content.Value()
	return func() tea.Msg {
		switch state.tab {
		case tabEnv:
			result, err := app.AddEnv(m.ctx, app.AddParams{Folder: folder, Content: content}, m.reporter)
			if err != nil {
				return operationResultMsg{err: err}
			}
			return operationResultMsg{message: fmt.Sprintf("Env note saved (%s).", shortID(result.ItemID)), reload: true}
		case tabNotes:
			result, err := app.EditNote(m.ctx, app.EditParams{Folder: folder, NameOrID: state.item.ID, EditNote: editContent(content)}, m.reporter)
			if err != nil {
				return operationResultMsg{err: err}
			}
			if !result.Updated {
				return operationResultMsg{message: "No changes made.", reload: false}
			}
			return operationResultMsg{message: fmt.Sprintf("Note '%s' saved.", result.Name), reload: true}
		default:
			return operationResultMsg{err: fmt.Errorf("%s does not support text editing", tabName(state.tab))}
		}
	}
}

func (m Model) saveSSHWizardCmd(state sshWizardState) tea.Cmd {
	folder := m.folderName()
	return func() tea.Msg {
		publicKey := strings.TrimSpace(state.publicInput.Value())
		if publicKey == "" {
			publicKey = state.derivedPub
		}
		result, err := app.AddSSHKey(m.ctx, app.AddSSHKeyParams{
			Folder:      folder,
			KeyName:     strings.TrimSpace(state.nameInput.Value()),
			OpenSSHKey:  state.openSSHKey,
			PublicKey:   publicKey,
			Fingerprint: state.fingerprint,
		}, m.reporter)
		if err != nil {
			return operationResultMsg{err: err}
		}
		return operationResultMsg{message: fmt.Sprintf("SSH key added (%s).", shortID(result.ItemID)), reload: true}
	}
}

func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.focus {
	case focusFolders:
		if len(m.folders) == 0 {
			return *m, nil
		}
		m.applySelectedFolderFromCache()
		m.focus = focusResources
		m.loading = false
		m.err = nil
		return *m, nil
	case focusResources:
		if len(m.currentItems()) > 0 || m.tab == tabEnv && m.resources.EnvNote != nil {
			m.focus = focusDetail
		}
	}
	return *m, nil
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		state := m.confirm
		m.interaction = interactionNone
		m.invalidateLoads()
		m.loading = true
		m.err = nil
		if state.kind == confirmRemove {
			m.status = "Removing..."
			return m, m.removeCmd(state)
		}
		m.status = "Cleaning..."
		return m, m.cleanCmd(state)
	case "n", "N", "esc":
		m.interaction = interactionNone
		m.status = "Canceled."
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.escape) {
		m.interaction = interactionNone
		m.status = "Edit canceled."
		return m, nil
	}
	if key.Matches(msg, m.keys.save) {
		m.invalidateLoads()
		m.loading = true
		m.err = nil
		m.status = "Saving..."
		state := m.edit
		m.interaction = interactionNone
		return m, m.saveEditCmd(state)
	}
	m.edit.content = m.edit.content.Update(msg)
	return m, nil
}

func (m Model) handleSSHWizardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.escape) {
		m.interaction = interactionNone
		m.status = "SSH add canceled."
		return m, nil
	}
	if m.sshWizard.step == sshStepConfirm {
		switch msg.String() {
		case "y", "Y", "ctrl+s":
			state := m.sshWizard
			m.interaction = interactionNone
			m.invalidateLoads()
			m.loading = true
			m.err = nil
			m.status = "Creating SSH key..."
			return m, m.saveSSHWizardCmd(state)
		case "n", "N":
			m.interaction = interactionNone
			m.status = "SSH add canceled."
			return m, nil
		default:
			return m, nil
		}
	}
	if key.Matches(msg, m.keys.enter) || key.Matches(msg, m.keys.save) {
		return m.advanceSSHWizard()
	}
	switch m.sshWizard.step {
	case sshStepPrivatePath:
		m.sshWizard.privateInput = m.sshWizard.privateInput.Update(msg)
	case sshStepPublicKey:
		m.sshWizard.publicInput = m.sshWizard.publicInput.Update(msg)
	case sshStepKeyName:
		m.sshWizard.nameInput = m.sshWizard.nameInput.Update(msg)
	}
	return m, nil
}

func (m Model) advanceSSHWizard() (tea.Model, tea.Cmd) {
	m.sshWizard.err = ""
	switch m.sshWizard.step {
	case sshStepPrivatePath:
		privatePath, openSSHKey, publicKey, fingerprint, err := parsePrivateKeyPath(m.sshWizard.privateInput.Value())
		if err != nil {
			m.sshWizard.err = err.Error()
			return m, nil
		}
		m.sshWizard.privateInput.SetValue(privatePath)
		m.sshWizard.openSSHKey = openSSHKey
		m.sshWizard.derivedPub = publicKey
		m.sshWizard.fingerprint = fingerprint
		if strings.TrimSpace(m.sshWizard.publicInput.Value()) == "" {
			m.sshWizard.publicInput.SetValue(publicKey)
		}
		m.sshWizard.step = sshStepPublicKey
	case sshStepPublicKey:
		m.sshWizard.step = sshStepKeyName
	case sshStepKeyName:
		if strings.TrimSpace(m.sshWizard.nameInput.Value()) == "" {
			m.sshWizard.err = "Key name cannot be empty."
			return m, nil
		}
		m.sshWizard.step = sshStepConfirm
	}
	return m, nil
}

func (m Model) startAdd() (tea.Model, tea.Cmd) {
	if m.currentFolder == nil || m.tab != tabSSH {
		m.status = "Add is currently available for SSH keys."
		return m, nil
	}
	m.interaction = interactionSSHWizard
	m.sshWizard = newSSHWizardState()
	m.status = "Adding SSH key."
	return m, nil
}

func (m Model) startEdit() (tea.Model, tea.Cmd) {
	if m.currentFolder == nil {
		return m, nil
	}
	if m.tab != tabEnv && m.tab != tabNotes {
		m.status = "Edit is available for env and note items."
		return m, nil
	}
	item, ok := m.currentItem()
	if !ok && m.tab == tabEnv {
		item = bwtypes.Item{Name: bwtypes.ReservedEnvNoteName}
		ok = true
	}
	if !ok {
		m.status = "No item selected."
		return m, nil
	}
	buffer := newTextBuffer(item.Notes)
	m.edit = editState{tab: m.tab, item: item, content: buffer}
	m.interaction = interactionEdit
	m.resizeInputs()
	m.status = fmt.Sprintf("Editing %s.", item.Name)
	return m, nil
}

func (m Model) startRemoveConfirm() (tea.Model, tea.Cmd) {
	if m.currentFolder == nil || (m.tab == tabEnv && m.resources.EnvNote == nil) {
		m.status = "No item selected."
		return m, nil
	}
	item, ok := m.currentItem()
	if !ok {
		m.status = "No item selected."
		return m, nil
	}
	m.confirm = confirmState{kind: confirmRemove, tab: m.tab, item: item}
	m.interaction = interactionConfirm
	m.status = "Confirm remove."
	return m, nil
}

func (m Model) startCleanConfirm() (tea.Model, tea.Cmd) {
	if m.currentFolder == nil {
		return m, nil
	}
	m.confirm = confirmState{kind: confirmClean, tab: m.tab}
	m.interaction = interactionConfirm
	m.status = "Confirm clean."
	return m, nil
}

func (m *Model) moveSelection(delta int) {
	if m.focus == focusFolders {
		if len(m.folders) == 0 {
			return
		}
		m.selectedFolder = clamp(m.selectedFolder+delta, len(m.folders))
		m.applySelectedFolderFromCache()
		m.status = fmt.Sprintf("Selected %s: %d SSH, env %s, %d note(s).", m.resources.Folder.Name, len(m.resources.SSHKeys), yesNo(m.resources.EnvNote != nil), len(m.resources.Notes))
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

func (m *Model) beginLoad() uint64 {
	m.loadSeq++
	m.activeLoadID = m.loadSeq
	return m.activeLoadID
}

func (m *Model) invalidateLoads() {
	m.loadSeq++
	m.activeLoadID = m.loadSeq
}

func (m *Model) applySelectedFolderFromCache() {
	if len(m.folders) == 0 {
		m.currentFolder = nil
		m.resources = app.BrowseResources{}
		return
	}

	m.selectedFolder = clamp(m.selectedFolder, len(m.folders))
	folder := m.folders[m.selectedFolder]
	m.currentFolder = &folder
	if resources, ok := m.resourcesByFolderID[folder.ID]; ok {
		m.resources = resources
	} else {
		m.resources = app.BrowseResources{Folder: folder}
	}
	for tab := range m.selectedItem {
		m.selectedItem[tab] = 0
	}
	m.clampSelection()
}

func selectFolderIndex(folders []bwtypes.Folder, previousID string, fallback int) int {
	if previousID != "" {
		for i, folder := range folders {
			if folder.ID == previousID {
				return i
			}
		}
	}
	return clamp(fallback, len(folders))
}

func (m *Model) resizeInputs() {
	width := m.width - 12
	if width < 40 {
		width = 80
	}
	m.edit.content.SetWidth(width)
	m.edit.content.SetHeight(12)
	m.sshWizard.privateInput.SetWidth(width)
	m.sshWizard.publicInput.SetWidth(width)
	m.sshWizard.nameInput.SetWidth(width)
}

func (m Model) folderName() string {
	if m.currentFolder == nil {
		return ""
	}
	return m.currentFolder.Name
}

type textBuffer struct {
	lines  []string
	row    int
	col    int
	width  int
	height int
}

func newTextBuffer(value string) textBuffer {
	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	return textBuffer{lines: lines, width: 80, height: 12}
}

func (b *textBuffer) SetWidth(width int) {
	if width > 0 {
		b.width = width
	}
}

func (b *textBuffer) SetHeight(height int) {
	if height > 0 {
		b.height = height
	}
}

func (b *textBuffer) SetValue(value string) {
	width, height := b.width, b.height
	*b = newTextBuffer(value)
	b.width = width
	b.height = height
}

func (b textBuffer) Value() string {
	return strings.Join(b.lines, "\n")
}

func (b textBuffer) View() string {
	lines := append([]string(nil), b.lines...)
	if len(lines) == 0 {
		lines = []string{""}
	}
	row := clamp(b.row, len(lines))
	col := b.col
	if col > len(lines[row]) {
		col = len(lines[row])
	}
	lines[row] = lines[row][:col] + "▌" + lines[row][col:]
	if b.height > 0 && len(lines) > b.height {
		start := row - b.height + 1
		if start < 0 {
			start = 0
		}
		lines = lines[start:min(start+b.height, len(lines))]
	}
	for i, line := range lines {
		if b.width > 0 {
			line = truncate(line, b.width)
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func (b textBuffer) Update(msg tea.KeyMsg) textBuffer {
	if len(b.lines) == 0 {
		b.lines = []string{""}
	}
	switch msg.Type {
	case tea.KeyRunes:
		b.insert(msg.String())
	case tea.KeySpace:
		b.insert(" ")
	case tea.KeyEnter:
		line := b.lines[b.row]
		before, after := line[:b.col], line[b.col:]
		b.lines[b.row] = before
		b.lines = append(b.lines[:b.row+1], append([]string{after}, b.lines[b.row+1:]...)...)
		b.row++
		b.col = 0
	case tea.KeyBackspace:
		if b.col > 0 {
			line := b.lines[b.row]
			b.lines[b.row] = line[:b.col-1] + line[b.col:]
			b.col--
		} else if b.row > 0 {
			prevLen := len(b.lines[b.row-1])
			b.lines[b.row-1] += b.lines[b.row]
			b.lines = append(b.lines[:b.row], b.lines[b.row+1:]...)
			b.row--
			b.col = prevLen
		}
	case tea.KeyDelete:
		line := b.lines[b.row]
		if b.col < len(line) {
			b.lines[b.row] = line[:b.col] + line[b.col+1:]
		} else if b.row < len(b.lines)-1 {
			b.lines[b.row] += b.lines[b.row+1]
			b.lines = append(b.lines[:b.row+1], b.lines[b.row+2:]...)
		}
	case tea.KeyLeft:
		if b.col > 0 {
			b.col--
		} else if b.row > 0 {
			b.row--
			b.col = len(b.lines[b.row])
		}
	case tea.KeyRight:
		if b.col < len(b.lines[b.row]) {
			b.col++
		} else if b.row < len(b.lines)-1 {
			b.row++
			b.col = 0
		}
	case tea.KeyUp:
		if b.row > 0 {
			b.row--
			b.col = min(b.col, len(b.lines[b.row]))
		}
	case tea.KeyDown:
		if b.row < len(b.lines)-1 {
			b.row++
			b.col = min(b.col, len(b.lines[b.row]))
		}
	}
	return b
}

func (b *textBuffer) insert(value string) {
	line := b.lines[b.row]
	b.lines[b.row] = line[:b.col] + value + line[b.col:]
	b.col += len(value)
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
		add:    key.NewBinding(key.WithKeys("a")),
		edit:   key.NewBinding(key.WithKeys("e")),
		delete: key.NewBinding(key.WithKeys("d")),
		clean:  key.NewBinding(key.WithKeys("c")),
		unlock: key.NewBinding(key.WithKeys("u")),
		save:   key.NewBinding(key.WithKeys("ctrl+s")),
		quit:   key.NewBinding(key.WithKeys("q")),
		ctrlC:  key.NewBinding(key.WithKeys("ctrl+c")),
	}
}

func newSSHWizardState() sshWizardState {
	privateInput := newTextBuffer("")
	privateInput.SetHeight(1)
	publicInput := newTextBuffer("")
	publicInput.SetHeight(1)
	nameInput := newTextBuffer("")
	nameInput.SetHeight(1)

	return sshWizardState{
		step:         sshStepPrivatePath,
		privateInput: privateInput,
		publicInput:  publicInput,
		nameInput:    nameInput,
	}
}

func parsePrivateKeyPath(value string) (expandedPath, openSSHKey, publicKey, fingerprint string, err error) {
	privatePath := strings.TrimSpace(value)
	if privatePath == "" {
		return "", "", "", "", fmt.Errorf("private key path is required")
	}
	expanded, err := pathutil.ExpandTilde(privatePath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("resolve private key path: %w", err)
	}
	if _, err := os.Stat(expanded); err != nil {
		return "", "", "", "", fmt.Errorf("private key file not found: %s", expanded)
	}
	privateKeyBytes, err := os.ReadFile(expanded)
	if err != nil {
		return "", "", "", "", fmt.Errorf("read private key: %w", err)
	}
	openSSHKey, publicKey, fingerprint, err = pkvkey.ParseAndConvertKey(privateKeyBytes)
	if err != nil {
		return "", "", "", "", fmt.Errorf("parse private key: %w", err)
	}
	return expanded, openSSHKey, publicKey, fingerprint, nil
}

func editContent(content string) app.EditSecureNoteFunc {
	return func(client *bw.Client, session string, item bwtypes.Item) (bool, error) {
		if item.Notes == content {
			return false, nil
		}
		if err := securenote.UpdateContent(client, session, item.ID, content); err != nil {
			return false, err
		}
		return true, nil
	}
}

func tabKind(tab resourceTab) string {
	switch tab {
	case tabSSH:
		return "ssh"
	case tabEnv:
		return "env"
	case tabNotes:
		return "note"
	default:
		return ""
	}
}

func idsForAction(tab resourceTab, item bwtypes.Item) []string {
	if tab == tabEnv || item.ID == "" {
		return nil
	}
	return []string{item.ID}
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
