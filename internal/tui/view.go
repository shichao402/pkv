package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	bwtypes "github.com/shichao402/pkv/internal/bw/types"
)

var (
	appStyle      = lipgloss.NewStyle().Padding(1, 2)
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	focusedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	subtleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	tabStyle      = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("245"))
	activeTab     = tabStyle.Copy().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(true)
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
	left := panelStyle.Width(leftWidth).Render(renderFolderList(m))
	right := panelStyle.Width(rightWidth).Render(renderResources(m))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
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
	}
	return titleStyle.Render(strings.Join(parts, " › "))
}

func renderFolderList(m Model) string {
	var b strings.Builder
	title := "Folders"
	if m.focus == focusFolders {
		title = focusedStyle.Render(title)
	}
	b.WriteString(title)
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
		cursor := "  "
		line := folder.Name
		if i == m.selectedFolder {
			cursor = "▸ "
			line = selectedStyle.Render(line)
		}
		b.WriteString(cursor)
		b.WriteString(line)
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
	}
	if m.focus == focusDetail {
		return renderDetail(m)
	}

	var b strings.Builder
	title := "Resources"
	if m.focus == focusResources {
		title = focusedStyle.Render(title)
	}
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(renderTabs(m.tab))
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
		b.WriteString("\n\n")
		b.WriteString(renderResourceHints(m))
		return b.String()
	}

	selected := m.selectedItem[m.tab]
	for i, item := range items {
		cursor := "  "
		line := renderResourceLine(item)
		if i == selected {
			cursor = "▸ "
			line = selectedStyle.Render(line)
		}
		b.WriteString(cursor)
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(renderResourceHints(m))
	return strings.TrimRight(b.String(), "\n")
}

func renderTabs(active resourceTab) string {
	names := []resourceTab{tabSSH, tabEnv, tabNotes}
	parts := make([]string, 0, len(names))
	for _, tab := range names {
		label := tabName(tab)
		if tab == active {
			parts = append(parts, activeTab.Render(label))
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
	b.WriteString(fmt.Sprintf("Name: %s\n", item.Name))
	b.WriteString(fmt.Sprintf("ID:   %s\n", item.ID))

	switch m.tab {
	case tabSSH:
		if item.SSHKey != nil {
			b.WriteString(fmt.Sprintf("Fingerprint: %s\n", item.SSHKey.KeyFingerprint))
			b.WriteString(fmt.Sprintf("Public key:   %s\n", truncate(item.SSHKey.PublicKey, 72)))
		} else {
			b.WriteString(subtleStyle.Render("No SSH key payload returned."))
			b.WriteString("\n")
		}
	case tabEnv:
		keys := envKeys(item.Notes)
		b.WriteString(fmt.Sprintf("Keys: %d\n", len(keys)))
		for _, key := range keys {
			b.WriteString("  ")
			b.WriteString(key)
			b.WriteString("\n")
		}
	case tabNotes:
		b.WriteString(fmt.Sprintf("Lines: %d\n", countLines(item.Notes)))
		b.WriteString("Preview:\n")
		b.WriteString(indent(truncate(item.Notes, 240)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(renderDetailHints(m))
	return b.String()
}

func renderConfirm(m Model) string {
	var b strings.Builder
	kind := tabKind(m.confirm.tab)
	title := "Confirm"
	verb := "clean local materialized resources for"
	target := kind
	if m.confirm.kind == confirmRemove {
		verb = "remove"
		target = m.confirm.item.Name
		if target == "" {
			target = shortID(m.confirm.item.ID)
		}
		title = "Confirm Remove"
	} else {
		title = "Confirm Clean"
	}
	b.WriteString(focusedStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Folder: %s\n", m.folderName()))
	b.WriteString(fmt.Sprintf("Action: %s %s\n", verb, target))
	if m.confirm.kind == confirmRemove && m.confirm.item.ID != "" {
		b.WriteString(fmt.Sprintf("ID:     %s\n", m.confirm.item.ID))
	}
	b.WriteString("\n")
	b.WriteString(errorStyle.Render("This action changes Bitwarden or local materialized files."))
	b.WriteString("\n\n")
	b.WriteString(subtleStyle.Render("y confirm · n/esc cancel"))
	return b.String()
}

func renderEdit(m Model) string {
	var b strings.Builder
	b.WriteString(focusedStyle.Render(fmt.Sprintf("Edit %s", tabName(m.edit.tab))))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Folder: %s\n", m.folderName()))
	b.WriteString(fmt.Sprintf("Item:   %s\n", m.edit.item.Name))
	if m.edit.item.ID != "" {
		b.WriteString(fmt.Sprintf("ID:     %s\n", m.edit.item.ID))
	}
	b.WriteString("\n")
	b.WriteString(m.edit.content.View())
	b.WriteString("\n\n")
	b.WriteString(subtleStyle.Render("ctrl+s save · esc cancel"))
	return b.String()
}

func renderSSHWizard(m Model) string {
	var b strings.Builder
	b.WriteString(focusedStyle.Render("Add SSH Key"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Folder: %s\n\n", m.folderName()))
	if m.sshWizard.err != "" {
		b.WriteString(errorStyle.Render(m.sshWizard.err))
		b.WriteString("\n\n")
	}
	switch m.sshWizard.step {
	case sshStepPrivatePath:
		b.WriteString("Private key path\n")
		b.WriteString(m.sshWizard.privateInput.View())
		b.WriteString("\n\n")
		b.WriteString(subtleStyle.Render("enter next · esc cancel"))
	case sshStepPublicKey:
		b.WriteString("Public key (leave empty to use derived key)\n")
		b.WriteString(m.sshWizard.publicInput.View())
		b.WriteString("\n\n")
		b.WriteString(subtleStyle.Render(fmt.Sprintf("Derived fingerprint: %s\nenter next · esc cancel", m.sshWizard.fingerprint)))
	case sshStepKeyName:
		b.WriteString("Key name\n")
		b.WriteString(m.sshWizard.nameInput.View())
		b.WriteString("\n\n")
		b.WriteString(subtleStyle.Render("enter summary · esc cancel"))
	case sshStepConfirm:
		publicKey := strings.TrimSpace(m.sshWizard.publicInput.Value())
		if publicKey == "" {
			publicKey = m.sshWizard.derivedPub
		}
		b.WriteString("Summary\n\n")
		b.WriteString(fmt.Sprintf("Key name:    %s\n", strings.TrimSpace(m.sshWizard.nameInput.Value())))
		b.WriteString(fmt.Sprintf("Private key: %s\n", displayPath(m.sshWizard.privateInput.Value())))
		b.WriteString(fmt.Sprintf("Fingerprint: %s\n", m.sshWizard.fingerprint))
		b.WriteString(fmt.Sprintf("Public key:  %s\n", truncate(publicKey, 72)))
		b.WriteString("\n")
		b.WriteString(subtleStyle.Render("y/ctrl+s create · n/esc cancel"))
	}
	return b.String()
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
	return lipgloss.JoinVertical(lipgloss.Left, "", status, subtleStyle.Render("↑↓ navigate · enter select · tab/←→ switch · a add · e edit · d remove · c clean · u unlock · r reload · q quit"))
}

func renderResourceHints(m Model) string {
	if m.tab == tabSSH {
		return subtleStyle.Render("enter detail · a add ssh · d remove · c clean · tab switch · esc folders")
	}
	if m.tab == tabEnv {
		return subtleStyle.Render("enter detail · e edit/create env · d remove · c clean · tab switch · esc folders")
	}
	return subtleStyle.Render("enter detail · e edit note · d remove · c clean · tab switch · esc folders")
}

func renderDetailHints(m Model) string {
	switch m.tab {
	case tabSSH:
		return subtleStyle.Render("d remove · c clean · esc back · q quit")
	case tabEnv:
		return subtleStyle.Render("e edit · d remove · c clean · esc back · q quit")
	case tabNotes:
		return subtleStyle.Render("e edit · d remove · c clean · esc back · q quit")
	default:
		return subtleStyle.Render("esc back · q quit")
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

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	if max <= 1 {
		return value[:max]
	}
	return value[:max-1] + "…"
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
