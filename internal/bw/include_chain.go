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
	for _, f := range folders {
		name := f.Name
		if _, seen := idByName[name]; seen {
			dupNames[name] = true
			continue
		}
		idByName[name] = f.ID
	}

	// Collectors for aggregated pre-flight errors.
	missing := make(map[string]bool)       // referenced folder not in vault
	duplicate := make(map[string]bool)     // referenced folder has duplicate names
	tooManyNotes := make(map[string]error) // folder -> FindIncludeNote error

	fetch := func(folder string) (string, bool, error) {
		if dupNames[folder] {
			duplicate[folder] = true
			// Treat as leaf: resolver should continue and surface other
			// problems rather than abort at first sight.
			return "", false, nil
		}
		id, ok := idByName[folder]
		if !ok {
			missing[folder] = true
			return "", false, nil
		}
		items, err := listItems(id)
		if err != nil {
			return "", false, fmt.Errorf("list items for folder %q: %w", folder, err)
		}
		note, ok, err := include.FindIncludeNote(items)
		if err != nil {
			// Multiple pkv.include notes in one folder is a user error; record
			// and treat as leaf so we can aggregate.
			tooManyNotes[folder] = err
			return "", false, nil
		}
		if !ok {
			return "", false, nil
		}
		return note.Notes, true, nil
	}

	chain, resolveErr := include.Resolve(rootFolder, fetch)

	// Aggregate every pre-flight problem found during the walk.
	var issues []string
	if names := sortedKeys(missing); len(names) > 0 {
		issues = append(issues, fmt.Sprintf("missing folders: %s", strings.Join(names, ", ")))
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

	// Map the chain's folder names back to full types.Folder structs. The
	// root is guaranteed to be in the vault at this point (otherwise it
	// would have been reported as missing above and we would have returned).
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
