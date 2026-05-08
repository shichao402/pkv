package bw

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/include"
)

// LoadIncludeChain returns the ordered set of Bitwarden folders reachable from
// rootFolder through `pkv.include` declarations. The returned slice starts
// with the root folder at index 0 (matching include.Resolve's contract).
//
// The chain is built in two phases:
//   - Fetch the full folder list once and build a name→ID map. Duplicate
//     folder names are collected for aggregated reporting; they are not
//     usable because the resolver addresses folders by name.
//   - Walk the include graph via include.Resolve. Folders that appear in a
//     pkv.include body but do not exist in Bitwarden (or are duplicated)
//     are collected rather than failing fast, so the user sees every
//     misconfiguration in one error.
//
// If any folder-name lookups failed OR include.Resolve returned an error
// (cycle, depth, fetch), both sets of problems are combined into a single
// error so the caller can surface everything at once.
func (c *Client) LoadIncludeChain(session, rootFolder string) ([]types.Folder, error) {
	listFolders := func() ([]types.Folder, error) {
		return c.ListFolders(session)
	}
	listItems := func(folderID string) ([]types.Item, error) {
		return c.ListItems(session, folderID)
	}
	return loadIncludeChain(rootFolder, listFolders, listItems)
}

// listFoldersFunc and listItemsFunc decouple loadIncludeChain from the
// Bitwarden CLI wrapper for testing. Production callers inject
// Client.ListFolders / Client.ListItems.
type listFoldersFunc func() ([]types.Folder, error)
type listItemsFunc func(folderID string) ([]types.Item, error)

func loadIncludeChain(
	rootFolder string,
	listFolders listFoldersFunc,
	listItems listItemsFunc,
) ([]types.Folder, error) {
	rootFolder = strings.TrimSpace(rootFolder)
	if rootFolder == "" {
		return nil, fmt.Errorf("LoadIncludeChain: root folder must not be empty")
	}

	folders, err := listFolders()
	if err != nil {
		return nil, fmt.Errorf("list Bitwarden folders: %w", err)
	}

	// Build name -> ID map. Collect duplicates so a later lookup can report
	// them instead of silently picking one.
	idByName := make(map[string]string, len(folders))
	dupNames := make(map[string]bool)
	// canonByLower maps lowercase folder name -> all canonical (vault) names
	// that share that case-insensitive form. Used as a fallback when an exact
	// match is missing, mirroring Client.GetFolderID's two-pass behavior.
	canonByLower := make(map[string][]string, len(folders))
	for _, f := range folders {
		name := f.Name
		if _, seen := idByName[name]; seen {
			dupNames[name] = true
			continue
		}
		idByName[name] = f.ID
		lower := strings.ToLower(name)
		canonByLower[lower] = append(canonByLower[lower], name)
	}

	// Collectors for aggregated pre-flight errors.
	missing := make(map[string]bool)        // referenced folder not in vault
	duplicate := make(map[string]bool)      // referenced folder has duplicate names
	tooManyNotes := make(map[string]error)  // folder -> FindIncludeNote error
	ambiguousCI := make(map[string][]string) // user input -> candidate canonical names

	// resolveName implements the two-pass lookup: exact match first, then
	// case-insensitive fallback. Returns the canonical (vault) name and a
	// status:
	//   "exact"     - exact case-sensitive match
	//   "ci"        - unique case-insensitive match (canonical may differ from input)
	//   "missing"   - no match at all
	//   "ambiguous" - multiple distinct canonical names share the lowercased form
	resolveName := func(name string) (canonical string, status string, candidates []string) {
		if _, ok := idByName[name]; ok {
			return name, "exact", nil
		}
		cands := canonByLower[strings.ToLower(name)]
		switch len(cands) {
		case 0:
			return name, "missing", nil
		case 1:
			return cands[0], "ci", nil
		default:
			return name, "ambiguous", append([]string(nil), cands...)
		}
	}

	fetch := func(folder string) (string, bool, error) {
		// Note: by the time fetch is called for a non-root folder the caller
		// (this same fetch on an earlier hop) has already rewritten include
		// body lines to canonical names, so `folder` should already match an
		// idByName key. Running resolveName again is harmless and lets the
		// root call go through the same code path.
		canonical, status, cands := resolveName(folder)
		switch status {
		case "missing":
			missing[folder] = true
			return "", false, nil
		case "ambiguous":
			ambiguousCI[folder] = cands
			return "", false, nil
		}
		if dupNames[canonical] {
			duplicate[canonical] = true
			// Treat as leaf: resolver should continue and surface other
			// problems rather than abort at first sight.
			return "", false, nil
		}
		id := idByName[canonical]
		items, err := listItems(id)
		if err != nil {
			return "", false, fmt.Errorf("list items for folder %q: %w", canonical, err)
		}
		note, ok, err := include.FindIncludeNote(items)
		if err != nil {
			// Multiple pkv.include notes in one folder is a user error; record
			// and treat as leaf so we can aggregate.
			tooManyNotes[canonical] = err
			return "", false, nil
		}
		if !ok {
			return "", false, nil
		}
		// Rewrite each declared include line to its canonical (vault) name so
		// include.Resolve sees normalized names. Lines that don't resolve are
		// kept verbatim so the next fetch hop will record them as missing /
		// ambiguous via the same resolveName logic.
		lines := include.ParseLines(note.Notes)
		for i, line := range lines {
			if canon, status, _ := resolveName(line); status == "exact" || status == "ci" {
				lines[i] = canon
			}
		}
		return strings.Join(lines, "\n"), true, nil
	}

	// Normalize the root before handing it to Resolve so the chain's first
	// element uses the vault canonical name.
	rootCanonical, rootStatus, rootCands := resolveName(rootFolder)
	switch rootStatus {
	case "missing":
		missing[rootFolder] = true
	case "ambiguous":
		ambiguousCI[rootFolder] = rootCands
	}

	chain, resolveErr := include.Resolve(rootCanonical, fetch)

	// Aggregate every pre-flight problem found during the walk.
	var issues []string
	if names := sortedKeys(missing); len(names) > 0 {
		issues = append(issues, fmt.Sprintf("missing folders: %s", strings.Join(names, ", ")))
	}
	if names := sortedMapKeys2(ambiguousCI); len(names) > 0 {
		for _, name := range names {
			issues = append(issues, fmt.Sprintf(
				"ambiguous folder name %q matches multiple vault folders by case: %s (rename so cases are unique)",
				name, strings.Join(ambiguousCI[name], ", "),
			))
		}
	}
	if names := sortedKeys(duplicate); len(names) > 0 {
		issues = append(issues,
			fmt.Sprintf("duplicate folder names in vault: %s (rename so each is unique)",
				strings.Join(names, ", ")))
	}
	if len(tooManyNotes) > 0 {
		for _, name := range sortedMapKeys(tooManyNotes) {
			issues = append(issues, fmt.Sprintf("folder %q: %v", name, tooManyNotes[name]))
		}
	}

	switch {
	case resolveErr != nil && len(issues) > 0:
		return nil, fmt.Errorf(
			"pkv.include chain aborted:\n- %s\n- resolver error: %v",
			strings.Join(issues, "\n- "),
			resolveErr,
		)
	case resolveErr != nil:
		return nil, resolveErr
	case len(issues) > 0:
		return nil, fmt.Errorf(
			"pkv.include chain aborted due to %d issue(s):\n- %s",
			len(issues),
			strings.Join(issues, "\n- "),
		)
	}

	// Map the chain's folder names back to full types.Folder structs. Names
	// in chain are already canonical because fetch rewrites include-body
	// lines and the root was normalized above.
	out := make([]types.Folder, 0, len(chain))
	for _, name := range chain {
		out = append(out, types.Folder{ID: idByName[name], Name: name})
	}
	return out, nil
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedMapKeys(m map[string]error) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedMapKeys2(m map[string][]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
