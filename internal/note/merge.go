// Package note merge logic for pkv.include chain.
//
// MergeNoteItems walks an include chain (chain[0] = current folder, rest in
// pkv.include declaration order) and selects, per note name, the item from the
// earliest folder to declare that name. Later declarations of the same name
// are recorded as shadowed so the caller can surface them in a conflict
// pre-flight block without aborting the sync.
//
// Semantics:
//   - Current folder wins: chain[0]'s notes override same-named notes from any
//     included folder.
//   - Includes provide defaults: notes only declared by included folders are
//     added to the merged set.
//   - Multi-include ordered by declaration: earlier folders in the chain win
//     over later ones when names collide.
//   - Conflicts are non-fatal: reported separately; merge always returns the
//     winner set.
//   - Reserved metadata (pkv.env, pkv.include) must be filtered out by the
//     caller — merge defensively drops them if encountered.
package note

import (
	"sort"

	"github.com/shichao402/pkv/internal/bw/types"
)

// SourcedNote pairs a merged note item with the folder that contributed it.
type SourcedNote struct {
	Item   types.Item
	Source string
}

// NoteConflict records a note name claimed by the winner plus the folders
// whose same-named notes were shadowed.
type NoteConflict struct {
	Name   string
	Winner string
	Losers []string
}

// MergedNotes is the result of walking an include chain and merging same-named
// notes across folders.
type MergedNotes struct {
	Items     []SourcedNote
	Conflicts []NoteConflict
	Chain     []string
}

// MergeNoteItems merges config notes across an include chain.
//
// chain[0] must be the current folder; remaining entries follow pkv.include
// declaration order. itemsByFolder maps folder name → already-filtered config
// notes (the caller is expected to have applied FilterConfigNotes, so
// pkv.env / pkv.include are absent). Missing entries in the map are treated
// as empty (folder has no config notes).
//
// The returned Items slice is sorted by note name for deterministic output;
// Conflicts is sorted by note name as well, and each Conflict.Losers list is
// in chain declaration order.
func MergeNoteItems(chain []string, itemsByFolder map[string][]types.Item) MergedNotes {
	chainCopy := append([]string(nil), chain...)

	type slot struct {
		item   types.Item
		source string
		losers []string
	}
	winners := make(map[string]*slot)

	for _, folder := range chain {
		items := itemsByFolder[folder]
		for _, item := range items {
			// Defensive: never let reserved metadata notes leak into the
			// merged config-note set even if a caller forgot to filter.
			if item.Name == types.ReservedEnvNoteName || item.Name == types.ReservedIncludeNoteName {
				continue
			}
			name := item.Name
			if existing, ok := winners[name]; ok {
				existing.losers = append(existing.losers, folder)
				continue
			}
			winners[name] = &slot{item: item, source: folder}
		}
	}

	names := make([]string, 0, len(winners))
	for name := range winners {
		names = append(names, name)
	}
	sort.Strings(names)

	merged := MergedNotes{
		Items:     make([]SourcedNote, 0, len(names)),
		Conflicts: nil,
		Chain:     chainCopy,
	}
	for _, name := range names {
		s := winners[name]
		merged.Items = append(merged.Items, SourcedNote{Item: s.item, Source: s.source})
		if len(s.losers) > 0 {
			merged.Conflicts = append(merged.Conflicts, NoteConflict{
				Name:   name,
				Winner: s.source,
				Losers: append([]string(nil), s.losers...),
			})
		}
	}
	sort.Slice(merged.Conflicts, func(i, j int) bool {
		return merged.Conflicts[i].Name < merged.Conflicts[j].Name
	})
	return merged
}
