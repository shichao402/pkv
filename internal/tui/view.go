package tui

import (
	"fmt"
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
	b.WriteString(subtleStyle.Render("enter detail · tab switch · esc folders"))
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
	b.WriteString(subtleStyle.Render("esc back · q quit"))
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
	return lipgloss.JoinVertical(lipgloss.Left, "", status, subtleStyle.Render("↑↓ navigate · enter select · tab/←→ switch · r reload · q quit"))
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
