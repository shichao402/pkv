package tui

import (
	"fmt"
	"strings"

	"github.com/shichao402/pkv/internal/app"
	bwtypes "github.com/shichao402/pkv/internal/bw/types"
)

type resourceDisplayItem struct {
	Item         bwtypes.Item
	SourceFolder string
	Direct       bool
	Label        string
}

func displayItemsForTab(tab resourceTab, res app.BrowseResources) []resourceDisplayItem {
	switch tab {
	case tabSSH:
		out := make([]resourceDisplayItem, 0, len(res.SSHKeys))
		for _, item := range res.SSHKeys {
			out = append(out, resourceDisplayItem{
				Item:   item,
				Direct: true,
				Label:  item.Name,
			})
		}
		return out
	case tabEnv:
		if len(res.ResolvedEnv) > 0 {
			out := make([]resourceDisplayItem, 0, len(res.ResolvedEnv))
			for _, v := range res.ResolvedEnv {
				out = append(out, resourceDisplayItem{
					Item: bwtypes.Item{
						Name:  v.Key,
						Notes: fmt.Sprintf("%s=%s", v.Key, v.Value),
					},
					SourceFolder: v.Source,
					Direct:       isOwnedByFolder(v.Source, res.Folder.Name),
					Label:        v.Key,
				})
			}
			return out
		}
		if res.EnvNote != nil {
			return []resourceDisplayItem{{
				Item:   *res.EnvNote,
				Direct: true,
				Label:  res.EnvNote.Name,
			}}
		}
		return nil
	case tabNotes:
		if len(res.ResolvedNotes) > 0 {
			out := make([]resourceDisplayItem, 0, len(res.ResolvedNotes))
			for _, sn := range res.ResolvedNotes {
				out = append(out, resourceDisplayItem{
					Item:         sn.Item,
					SourceFolder: sn.Source,
					Direct:       isOwnedByFolder(sn.Source, res.Folder.Name),
					Label:        sn.Item.Name,
				})
			}
			return out
		}
		out := make([]resourceDisplayItem, 0, len(res.Notes))
		for _, item := range res.Notes {
			out = append(out, resourceDisplayItem{
				Item:   item,
				Direct: true,
				Label:  item.Name,
			})
		}
		return out
	default:
		return nil
	}
}

func isOwnedByFolder(source, folderName string) bool {
	return source == "" || source == folderName
}

func formatResourceLine(display resourceDisplayItem) string {
	name := display.Label
	if name == "" {
		name = display.Item.Name
	}
	if name == "" {
		name = "(unnamed)"
	}
	line := fmt.Sprintf("%s  %s", shortID(display.Item.ID), name)
	if display.SourceFolder != "" && !display.Direct {
		line += fmt.Sprintf("  [from: %s]", display.SourceFolder)
	}
	return line
}

func formatFolderStatus(res app.BrowseResources) string {
	envCount := len(res.ResolvedEnv)
	if envCount == 0 && res.EnvNote != nil {
		envCount = len(envKeys(res.EnvNote.Notes))
	}
	noteCount := len(res.ResolvedNotes)
	if noteCount == 0 {
		noteCount = len(res.Notes)
	}
	status := fmt.Sprintf(
		"Selected %s: %d SSH, %d env key(s), %d note(s).",
		res.Folder.Name,
		len(res.SSHKeys),
		envCount,
		noteCount,
	)
	if len(res.IncludeChain) > 1 {
		status += fmt.Sprintf(" include: %s", strings.Join(res.IncludeChain, " -> "))
	}
	if res.ResolveErr != nil {
		status += fmt.Sprintf(" (include: %s)", res.ResolveErr.Error())
	}
	return status
}

func includeBanner(res app.BrowseResources) string {
	if len(res.DirectIncludes) == 0 {
		return ""
	}
	if len(res.IncludeChain) > 1 {
		return subtleStyle.Render(fmt.Sprintf("Includes: %s", strings.Join(res.IncludeChain, " -> ")))
	}
	return subtleStyle.Render(fmt.Sprintf("Includes: %s", strings.Join(res.DirectIncludes, ", ")))
}

func sshIncludeHint(res app.BrowseResources) string {
	if len(res.IncludeChain) <= 1 {
		return ""
	}
	return subtleStyle.Render("SSH is not expanded through pkv.include.")
}
