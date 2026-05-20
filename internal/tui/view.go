package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	bwtypes "github.com/shichao402/pkv/internal/bw/types"
)

var (
	appStyle           = lipgloss.NewStyle().Padding(1, 2)
	titleStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	focusedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	subtleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	panelStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	focusedPanelStyle  = panelStyle.BorderForeground(lipgloss.Color("205"))
	inactivePanelStyle = panelStyle.BorderForeground(lipgloss.Color("240"))
	helpPanelStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Padding(1, 2)
	selectedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(true)
	inactiveSelection  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	tabStyle           = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("245"))
	activeTab          = tabStyle.Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(true)
)

func render(m Model) string {
	width := m.width
	if width < 80 {
		width = 100
	}
	contentWidth := width - 4
	leftWidth := contentWidth / 3
	rightWidth := contentWidth - leftWidth - 2

	breadcrumb := renderBreadcrumb(m)
	leftStyle := inactivePanelStyle
	if folderPaneFocused(m) {
		leftStyle = focusedPanelStyle
	}
	rightStyle := inactivePanelStyle
	if resourcePaneFocused(m) {
		rightStyle = focusedPanelStyle
	}
	left := leftStyle.Width(leftWidth).Render(renderFolderList(m))
	right := rightStyle.Width(rightWidth).Render(renderResources(m))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	if m.helpOpen {
		body = lipgloss.PlaceHorizontal(contentWidth, lipgloss.Center, renderHelpPopup(m, contentWidth))
	}
	footer := renderFooter(m)

	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, breadcrumb, body, footer))
}

func renderBreadcrumb(m Model) string {
	parts := []string{"PKV", "Folders"}
	if m.currentFolder != nil {
		parts = append(parts, m.currentFolder.Name)
	}
	if m.focus == focusDetail {
		parts = append(parts, tabName(m.tab), "Detail")
	}
	switch m.interaction {
	case interactionConfirm:
		parts = append(parts, "Confirm")
	case interactionEdit:
		parts = append(parts, "Edit")
	case interactionSSHWizard:
		parts = append(parts, "Add SSH")
	case interactionAddFolder:
		parts = append(parts, "Add Folder")
	}
	if m.helpOpen {
		parts = append(parts, "Help")
	}
	return titleStyle.Render(strings.Join(parts, " › "))
}

func folderPaneFocused(m Model) bool {
	return m.interaction == interactionNone && m.focus == focusFolders
}

func resourcePaneFocused(m Model) bool {
	return m.interaction != interactionNone || m.focus == focusResources || m.focus == focusDetail
}

func renderPaneTitle(title string, active bool) string {
	if active {
		return focusedStyle.Render("● " + title)
	}
	return subtleStyle.Render("○ " + title)
}

func renderSelectableLine(value string, selected bool, active bool) string {
	if !selected {
		return "  " + value
	}
	if active {
		return "▸ " + selectedStyle.Render(value)
	}
	return inactiveSelection.Render("▹ " + value)
}

func renderFolderList(m Model) string {
	var b strings.Builder
	active := folderPaneFocused(m)
	b.WriteString(renderPaneTitle("Folders", active))
	b.WriteString("\n\n")

	if len(m.folders) == 0 {
		if m.loading {
			b.WriteString(subtleStyle.Render("Loading folders..."))
		} else {
			b.WriteString(subtleStyle.Render("No folders."))
		}
		return b.String()
	}

	for i, folder := range m.folders {
		b.WriteString(renderSelectableLine(folder.Name, i == m.selectedFolder, active))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderResources(m Model) string {
	switch m.interaction {
	case interactionConfirm:
		return renderConfirm(m)
	case interactionEdit:
		return renderEdit(m)
	case interactionSSHWizard:
		return renderSSHWizard(m)
	case interactionAddFolder:
		return renderAddFolder(m)
	}
	if m.focus == focusDetail {
		return renderDetail(m)
	}

	var b strings.Builder
	active := m.interaction == interactionNone && m.focus == focusResources
	b.WriteString(renderPaneTitle("Resources", active))
	b.WriteString("\n")
	b.WriteString(renderTabs(m.tab, active))
	b.WriteString("\n\n")

	if m.currentFolder == nil {
		b.WriteString(subtleStyle.Render("Select a folder and press enter."))
		return b.String()
	}
	if m.loading {
		b.WriteString(subtleStyle.Render("Loading resources..."))
		return b.String()
	}
	if m.err != nil {
		b.WriteString(errorStyle.Render(m.err.Error()))
		return b.String()
	}

	items := m.currentItems()
	if len(items) == 0 {
		b.WriteString(subtleStyle.Render(fmt.Sprintf("No %s items.", strings.ToLower(tabName(m.tab)))))
		return b.String()
	}

	selected := m.selectedItem[m.tab]
	for i, item := range items {
		b.WriteString(renderSelectableLine(renderResourceLine(item), i == selected, active))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderTabs(active resourceTab, paneActive bool) string {
	names := []resourceTab{tabSSH, tabEnv, tabNotes}
	parts := make([]string, 0, len(names))
	for _, tab := range names {
		label := tabName(tab)
		if tab == active && paneActive {
			parts = append(parts, activeTab.Render(label))
		} else if tab == active {
			parts = append(parts, inactiveSelection.Render(label))
		} else {
			parts = append(parts, tabStyle.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func renderDetail(m Model) string {
	item, ok := m.currentItem()
	if !ok {
		return subtleStyle.Render("No item selected.")
	}

	var b strings.Builder
	b.WriteString(focusedStyle.Render(tabName(m.tab) + " Detail"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Name: %s\n", item.Name)
	fmt.Fprintf(&b, "ID:   %s\n", item.ID)

	switch m.tab {
	case tabSSH:
		if item.SSHKey != nil {
			fmt.Fprintf(&b, "Fingerprint: %s\n", item.SSHKey.KeyFingerprint)
			fmt.Fprintf(&b, "Public key:   %s\n", truncate(item.SSHKey.PublicKey, 72))
		} else {
			b.WriteString(subtleStyle.Render("No SSH key payload returned."))
			b.WriteString("\n")
		}
	case tabEnv:
		keys := envKeys(item.Notes)
		fmt.Fprintf(&b, "Keys: %d\n", len(keys))
		for _, key := range keys {
			b.WriteString("  ")
			b.WriteString(key)
			b.WriteString("\n")
		}
	case tabNotes:
		fmt.Fprintf(&b, "Lines: %d\n", countLines(item.Notes))
		b.WriteString("Preview:\n")
		b.WriteString(indent(truncate(item.Notes, 240)))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func renderConfirm(m Model) string {
	var b strings.Builder
	kind := tabKind(m.confirm.tab)
	title := "Confirm Clean"
	verb := "clean local materialized resources for"
	target := kind
	warning := "This action changes local materialized files."
	scope := localMaterializedScope(kind)

	switch m.confirm.kind {
	case confirmRemove:
		verb = "remove"
		target = m.confirm.item.Name
		if target == "" {
			target = shortID(m.confirm.item.ID)
		}
		title = "Confirm Remove"
		warning = "This action changes Bitwarden and local materialized files."
		scope = "Bitwarden item plus any tracked local materialized files"
	case confirmGet:
		title = "Confirm Get"
		verb = "get local materialized resources for"
	}

	b.WriteString(focusedStyle.Render(title))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Folder: %s\n", m.folderName())
	fmt.Fprintf(&b, "Action: %s %s\n", verb, target)
	fmt.Fprintf(&b, "Local:  %s\n", scope)
	if m.confirm.kind == confirmRemove && m.confirm.item.ID != "" {
		fmt.Fprintf(&b, "ID:     %s\n", m.confirm.item.ID)
	}
	b.WriteString("\n")
	b.WriteString(errorStyle.Render(warning))
	return b.String()
}

func renderEdit(m Model) string {
	var b strings.Builder
	b.WriteString(focusedStyle.Render(fmt.Sprintf("Edit %s", tabName(m.edit.tab))))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Folder: %s\n", m.folderName())
	fmt.Fprintf(&b, "Item:   %s\n", m.edit.item.Name)
	if m.edit.item.ID != "" {
		fmt.Fprintf(&b, "ID:     %s\n", m.edit.item.ID)
	}
	b.WriteString("\n")
	b.WriteString(m.edit.content.View())
	return strings.TrimRight(b.String(), "\n")
}

func renderAddFolder(m Model) string {
	var b strings.Builder
	b.WriteString(focusedStyle.Render("Add Folder"))
	b.WriteString("\n\n")
	if m.addFolder.err != "" {
		b.WriteString(errorStyle.Render(m.addFolder.err))
		b.WriteString("\n\n")
	}
	b.WriteString("Folder name\n")
	b.WriteString(m.addFolder.nameInput.View())
	return strings.TrimRight(b.String(), "\n")
}

func renderSSHWizard(m Model) string {
	var b strings.Builder
	b.WriteString(focusedStyle.Render("Add SSH Key"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Folder: %s\n\n", m.folderName())
	if m.sshWizard.err != "" {
		b.WriteString(errorStyle.Render(m.sshWizard.err))
		b.WriteString("\n\n")
	}
	switch m.sshWizard.step {
	case sshStepPrivatePath:
		b.WriteString("Private key path (leave empty to generate a new key)\n")
		b.WriteString(m.sshWizard.privateInput.View())
	case sshStepPublicKey:
		b.WriteString("Public key (leave empty to use derived key)\n")
		b.WriteString(m.sshWizard.publicInput.View())
		b.WriteString("\n\n")
		b.WriteString(subtleStyle.Render(fmt.Sprintf("Derived fingerprint: %s", m.sshWizard.fingerprint)))
	case sshStepKeyName:
		b.WriteString("Key name\n")
		b.WriteString(m.sshWizard.nameInput.View())
	case sshStepConfirm:
		publicKey := strings.TrimSpace(m.sshWizard.publicInput.Value())
		if publicKey == "" {
			publicKey = m.sshWizard.derivedPub
		}
		privateKey := displayPath(m.sshWizard.privateInput.Value())
		if m.sshWizard.generated {
			privateKey = "generated in memory"
		}
		b.WriteString("Summary\n\n")
		fmt.Fprintf(&b, "Key name:    %s\n", strings.TrimSpace(m.sshWizard.nameInput.Value()))
		fmt.Fprintf(&b, "Private key: %s\n", privateKey)
		fmt.Fprintf(&b, "Fingerprint: %s\n", m.sshWizard.fingerprint)
		fmt.Fprintf(&b, "Public key:  %s\n", truncate(publicKey, 72))
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderFooter(m Model) string {
	status := m.status
	if status == "" {
		status = "Ready."
	}
	if m.err != nil {
		status = errorStyle.Render(status)
	} else if m.loading {
		status = subtleStyle.Render(status)
	}
	return lipgloss.JoinVertical(lipgloss.Left, "", status, subtleStyle.Render(renderFooterHint(m)))
}

func renderFooterHint(m Model) string {
	if m.helpOpen {
		return "Help · ?/esc close · q quit"
	}
	switch m.interaction {
	case interactionConfirm:
		return "Confirm · y confirm · n/esc cancel · ? help"
	case interactionEdit:
		return fmt.Sprintf("Edit %s · ctrl+s save · esc cancel · ? help", tabName(m.edit.tab))
	case interactionAddFolder:
		return "Add Folder · enter/ctrl+s create · esc cancel · ? help"
	case interactionSSHWizard:
		return renderSSHWizardFooterHint(m)
	default:
		return renderNavigationFooterHint(m)
	}
}

func renderNavigationFooterHint(m Model) string {
	switch m.focus {
	case focusFolders:
		return "Folders · ↑↓ navigate · enter open resources · tab/→ resources · a add folder · u unlock · r reload · ? help · q quit"
	case focusDetail:
		return renderDetailFooterHint(m)
	default:
		return renderResourceFooterHint(m)
	}
}

func renderResourceFooterHint(m Model) string {
	parts := []string{tabName(m.tab), "↑↓ navigate", "enter detail", "tab/←→ switch tab", "g get"}
	switch m.tab {
	case tabSSH:
		parts = append(parts, "a add ssh")
	case tabEnv:
		parts = append(parts, "e edit/create env")
	case tabNotes:
		parts = append(parts, "e edit note")
	}
	parts = append(parts, "d remove", "c clean", "esc folders", "? help", "q quit")
	return strings.Join(parts, " · ")
}

func renderDetailFooterHint(m Model) string {
	parts := []string{tabName(m.tab) + " Detail", "g get"}
	if m.tab == tabEnv || m.tab == tabNotes {
		parts = append(parts, "e edit")
	}
	parts = append(parts, "d remove", "c clean", "esc back", "? help", "q quit")
	return strings.Join(parts, " · ")
}

func renderSSHWizardFooterHint(m Model) string {
	switch m.sshWizard.step {
	case sshStepConfirm:
		return "Add SSH · y/ctrl+s create · n/esc cancel · ? help"
	case sshStepKeyName:
		return "Add SSH · enter/ctrl+s summary · esc cancel · ? help"
	default:
		return "Add SSH · enter/ctrl+s next · esc cancel · ? help"
	}
}

func renderHelpPopup(m Model, contentWidth int) string {
	width := contentWidth - 12
	if width > 72 {
		width = 72
	}
	if width < 48 {
		width = 48
	}

	sections := []string{
		focusedStyle.Render("Keyboard Help"),
		"",
		"Global",
		"  ? help    q quit    u unlock    r reload",
		"",
		"Folders",
		"  ↑/↓ or j/k move    enter open resources    tab/→ focus resources    a add folder",
		"",
		"Resources",
		"  ↑/↓ or j/k move    enter detail    tab/←→ switch tab    esc folders",
		"  g get    d remove    c clean    a add ssh    e edit env/note",
		"",
		"Detail",
		"  esc back    g get    d remove    c clean    e edit env/note",
		"",
		"Forms and confirmations",
		"  ctrl+s save/create    y confirm    n/esc cancel",
	}
	return helpPanelStyle.Width(width).Render(strings.Join(sections, "\n"))
}

func localMaterializedScope(kind string) string {
	switch kind {
	case "ssh":
		return "~/.ssh key files and SSH config"
	case "env":
		return "~/.pkv/env env artifacts"
	case "note":
		return "current working directory note files"
	default:
		return "local materialized files"
	}
}

func renderResourceLine(item bwtypes.Item) string {
	name := item.Name
	if name == "" {
		name = "(unnamed)"
	}
	return fmt.Sprintf("%s  %s", shortID(item.ID), name)
}

func tabName(tab resourceTab) string {
	switch tab {
	case tabSSH:
		return "SSH"
	case tabEnv:
		return "Env"
	case tabNotes:
		return "Notes"
	default:
		return "Unknown"
	}
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func displayPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	if base == path {
		return path
	}
	return "…/" + base
}

func truncate(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxLen {
		return value
	}
	if maxLen <= 1 {
		return value[:maxLen]
	}
	return value[:maxLen-1] + "…"
}

func countLines(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	return len(strings.Split(value, "\n"))
}

func envKeys(notes string) []string {
	var keys []string
	for _, line := range strings.Split(notes, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func indent(value string) string {
	if value == "" {
		return "  " + subtleStyle.Render("(empty)")
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}
