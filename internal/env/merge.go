// Package env: merge.go wires the pkv.include chain into env deployment.
//
// The merge step is pure: given the chain (chain[0] is the current folder,
// followed by transitively resolved include targets in declaration order) and
// the raw `pkv.env` body per folder, it produces a deterministic, ordered
// list of SourcedEnvVar plus a list of detected EnvConflict.
//
// Merge semantics (MVP decision from #114):
//
//   - Current folder wins. The first folder in chain that defines a key owns
//     it; later folders only provide defaults.
//   - Multi-include order follows the pkv.include body (first line wins among
//     siblings). resolver.Resolve already returns chain in that order, so we
//     just iterate chain.
//   - Conflicts are collected and returned, never fatal. Callers are expected
//     to surface them in the pre-flight log so the user sees which include is
//     being overridden by whom.
//   - A folder missing `pkv.env` is skipped silently. It is legal for an
//     include target to exist only to provide notes.
//
// Parse errors in any folder's pkv.env are fatal and propagated with the
// folder name so the user can locate the bad note.
package env

import (
	"fmt"
	"sort"
)

// SourcedEnvVar is a merged key/value pair annotated with the folder that
// supplied it.
type SourcedEnvVar struct {
	EnvVar
	Source string
}

// EnvConflict describes one key that was defined in more than one folder on
// the include chain. Winner is the folder whose value survived; Losers are
// the other folders (in the order they appeared on the chain) that were
// shadowed.
type EnvConflict struct {
	Key    string
	Winner string
	Losers []string
}

// MergeResult aggregates the output of MergePkvEnvNotes.
//
// Vars is sorted by Key for stable downstream artifact output (matches the
// existing Deploy() ordering). Conflicts is sorted by Key for stable logs.
// Chain is returned as-is so callers can print the expansion trace without
// re-querying the resolver.
type MergeResult struct {
	Vars      []SourcedEnvVar
	Conflicts []EnvConflict
	Chain     []string
}

// MergePkvEnvNotes merges pkv.env bodies across the include chain.
//
// chain must be non-empty and chain[0] is treated as the current folder (its
// values take precedence over every include).
//
// notesByFolder maps folder name to the raw note body. A missing key or empty
// string is treated as "no pkv.env in that folder" and silently skipped.
//
// A parse error in any folder's body is returned wrapped with the folder name.
// In that case the returned MergeResult is zero-valued.
func MergePkvEnvNotes(chain []string, notesByFolder map[string]string) (MergeResult, error) {
	if len(chain) == 0 {
		return MergeResult{}, fmt.Errorf("merge pkv.env: empty include chain")
	}

	// winner[key] is the folder that owns key. sources[key] is the ordered
	// list of folders that *also* declared key (winner first, shadowed after).
	type slot struct {
		value  string
		source string
		losers []string
	}
	winner := make(map[string]*slot)
	// orderedKeys preserves first-seen order so the result is deterministic
	// even before we sort by key for the final slice.
	var orderedKeys []string

	for _, folder := range chain {
		body, ok := notesByFolder[folder]
		if !ok || body == "" {
			continue
		}
		vars, err := ParseEnvVars(body)
		if err != nil {
			return MergeResult{}, fmt.Errorf("parse pkv.env in folder %q: %w", folder, err)
		}
		for _, v := range vars {
			existing, seen := winner[v.Key]
			if !seen {
				winner[v.Key] = &slot{value: v.Value, source: folder}
				orderedKeys = append(orderedKeys, v.Key)
				continue
			}
			// chain[0] already wins (iterated first). Later folders are only
			// recorded as losers for conflict reporting.
			existing.losers = append(existing.losers, folder)
		}
	}

	// Build final vars sorted by key for stable artifact output.
	keys := append([]string(nil), orderedKeys...)
	sort.Strings(keys)
	vars := make([]SourcedEnvVar, 0, len(keys))
	for _, k := range keys {
		s := winner[k]
		vars = append(vars, SourcedEnvVar{
			EnvVar: EnvVar{Key: k, Value: s.value},
			Source: s.source,
		})
	}

	// Collect conflicts sorted by key so the log output is deterministic.
	var conflictKeys []string
	for k, s := range winner {
		if len(s.losers) > 0 {
			conflictKeys = append(conflictKeys, k)
		}
	}
	sort.Strings(conflictKeys)
	conflicts := make([]EnvConflict, 0, len(conflictKeys))
	for _, k := range conflictKeys {
		s := winner[k]
		conflicts = append(conflicts, EnvConflict{
			Key:    k,
			Winner: s.source,
			Losers: append([]string(nil), s.losers...),
		})
	}

	return MergeResult{
		Vars:      vars,
		Conflicts: conflicts,
		Chain:     append([]string(nil), chain...),
	}, nil
}
