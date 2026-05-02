// Package include implements the `pkv.include` resolution layer.
//
// A `pkv.include` Secure Note in a Bitwarden folder declares other folders whose
// env and note items should be merged into the current folder at read time.
// Syntax is intentionally minimal:
//
//   - One folder name per line.
//   - Leading and trailing whitespace is trimmed.
//   - Blank lines are ignored.
//   - Lines whose first non-whitespace character is `#` are treated as comments.
//
// Resolve walks the include graph starting from a root folder, following each
// `pkv.include` transitively. Duplicates are collapsed (first occurrence wins to
// preserve declaration order). The maximum transitive depth is MaxDepth; cycles
// are rejected. Both depth overflow and cycles are hard failures that include
// the full path so users can locate the misconfiguration.
//
// This package has no dependency on Bitwarden. Callers inject a FetchFunc that
// produces the raw `pkv.include` content for a given folder. The integration
// with bw.Client lives in a separate card.
package include

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/shichao402/pkv/internal/bw/types"
)

// MaxDepth is the maximum number of transitive include hops from the root
// folder. Root itself is depth 0; a folder listed in the root's pkv.include
// is depth 1. Anything beyond MaxDepth is rejected.
const MaxDepth = 3

// FetchFunc returns the raw `pkv.include` Secure Note body for the given folder.
//
// Contract:
//   - exists=false, err=nil: the folder exists but has no `pkv.include` note.
//     Resolve treats this as a leaf.
//   - exists=true, err=nil: `notes` holds the raw body (may be empty string).
//   - err != nil: Resolve aborts with the error wrapped and the current include
//     path attached so the user can see where resolution stopped.
//
// Folder-existence errors (unknown folder, access denied) should surface via
// err; Resolve does not attempt to distinguish them.
type FetchFunc func(folder string) (notes string, exists bool, err error)

// ParseLines returns the folder names declared in a pkv.include body, in
// declaration order. Blank lines and `#` comments are skipped. Each returned
// name is whitespace-trimmed. Duplicates within a single body are preserved
// (deduplication happens at the graph level in Resolve).
func ParseLines(notes string) []string {
	if notes == "" {
		return nil
	}
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(notes))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// FindIncludeNote returns the single pkv.include Secure Note in the given
// per-folder item list, if present. It mirrors bw.FindManagedEnvNote so
// callers have a consistent way to locate reserved-name notes.
//
// Multiple notes with the reserved name in one folder is a configuration
// error and produces an explicit error; Bitwarden does allow duplicate item
// names, so this check must be done client-side.
func FindIncludeNote(items []types.Item) (types.Item, bool, error) {
	var matches []types.Item
	for _, item := range items {
		if item.Type == types.ItemTypeSecureNote && item.Name == types.ReservedIncludeNoteName {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return types.Item{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return types.Item{}, false, fmt.Errorf(
			"found %d Secure Notes named %q in one folder; keep only one",
			len(matches),
			types.ReservedIncludeNoteName,
		)
	}
}

// Resolve walks the include graph rooted at `root` and returns the ordered
// list of folders whose contents should be merged at read time.
//
// The returned slice always starts with `root` (at index 0). Subsequent
// entries are the transitively-included folders in DFS order with duplicates
// collapsed. Callers use the order to apply "current folder wins, includes
// provide defaults" semantics: iterate in the returned order and let earlier
// entries win on name clashes.
//
// Failure modes:
//   - Cycle detected: returns error naming the full cyclic path.
//   - Chain exceeds MaxDepth: returns error naming the path that overflowed.
//   - FetchFunc returns an error: wrapped with the current resolution path.
//
// An empty or whitespace-only pkv.include note is treated as "no includes" —
// not an error.
func Resolve(root string, fetch FetchFunc) ([]string, error) {
	if fetch == nil {
		return nil, fmt.Errorf("include.Resolve: fetch callback must not be nil")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("include.Resolve: root folder name must not be empty")
	}

	seen := map[string]bool{root: true}
	chain := []string{root}
	stack := []string{root} // used to report cycle/depth paths
	if err := resolveWalk(root, 0, fetch, seen, &chain, &stack); err != nil {
		return nil, err
	}
	return chain, nil
}

func resolveWalk(
	current string,
	depth int,
	fetch FetchFunc,
	seen map[string]bool,
	chain *[]string,
	stack *[]string,
) error {
	notes, exists, err := fetch(current)
	if err != nil {
		return fmt.Errorf("fetch pkv.include for %q (path: %s): %w", current, formatPath(*stack), err)
	}
	if !exists {
		return nil
	}
	children := ParseLines(notes)
	for _, child := range children {
		// Cycle detection: the child appears somewhere on the active stack.
		if onStack(*stack, child) {
			return fmt.Errorf(
				"cycle detected in pkv.include chain: %s",
				formatPath(append(append([]string(nil), *stack...), child)),
			)
		}
		if seen[child] {
			// Already merged via an earlier branch; skip without recursing but
			// do not treat as a cycle. Declaration order is preserved by the
			// earlier visit.
			continue
		}
		childDepth := depth + 1
		if childDepth > MaxDepth {
			return fmt.Errorf(
				"pkv.include depth %d exceeds max %d at %s",
				childDepth,
				MaxDepth,
				formatPath(append(append([]string(nil), *stack...), child)),
			)
		}
		seen[child] = true
		*chain = append(*chain, child)
		*stack = append(*stack, child)
		if err := resolveWalk(child, childDepth, fetch, seen, chain, stack); err != nil {
			return err
		}
		*stack = (*stack)[:len(*stack)-1]
	}
	return nil
}

func onStack(stack []string, name string) bool {
	for _, s := range stack {
		if s == name {
			return true
		}
	}
	return false
}

func formatPath(path []string) string {
	return strings.Join(path, " -> ")
}
