package note

import (
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
)

func item(id, name, body string) types.Item {
	return types.Item{ID: id, Type: types.ItemTypeSecureNote, Name: name, Notes: body}
}

func TestMergeNoteItems_SingleFolderNoInclude(t *testing.T) {
	chain := []string{"root"}
	items := map[string][]types.Item{
		"root": {item("a", "app.env.json", "a"), item("b", "README.md", "b")},
	}

	got := MergeNoteItems(chain, items)

	if len(got.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got.Items))
	}
	if got.Items[0].Item.Name != "README.md" || got.Items[0].Source != "root" {
		t.Errorf("expected README.md from root first (sorted), got %+v", got.Items[0])
	}
	if got.Items[1].Item.Name != "app.env.json" || got.Items[1].Source != "root" {
		t.Errorf("expected app.env.json from root, got %+v", got.Items[1])
	}
	if len(got.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %+v", got.Conflicts)
	}
}

func TestMergeNoteItems_CurrentFolderWins(t *testing.T) {
	chain := []string{"root", "vikunja"}
	items := map[string][]types.Item{
		"root":    {item("r1", "app.env.json", "current")},
		"vikunja": {item("v1", "app.env.json", "shared")},
	}

	got := MergeNoteItems(chain, items)

	if len(got.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got.Items))
	}
	if got.Items[0].Item.ID != "r1" || got.Items[0].Source != "root" {
		t.Errorf("expected root to win, got source=%s id=%s", got.Items[0].Source, got.Items[0].Item.ID)
	}
	if len(got.Conflicts) != 1 || got.Conflicts[0].Name != "app.env.json" ||
		got.Conflicts[0].Winner != "root" || len(got.Conflicts[0].Losers) != 1 ||
		got.Conflicts[0].Losers[0] != "vikunja" {
		t.Errorf("expected conflict app.env.json winner=root losers=[vikunja], got %+v", got.Conflicts)
	}
}

func TestMergeNoteItems_IncludeProvidesDefault(t *testing.T) {
	chain := []string{"root", "vikunja"}
	items := map[string][]types.Item{
		"root":    {item("r1", "local.md", "r")},
		"vikunja": {item("v1", "shared.json", "v")},
	}

	got := MergeNoteItems(chain, items)

	if len(got.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got.Items))
	}
	byName := map[string]SourcedNote{}
	for _, it := range got.Items {
		byName[it.Item.Name] = it
	}
	if byName["local.md"].Source != "root" {
		t.Errorf("local.md source = %s, want root", byName["local.md"].Source)
	}
	if byName["shared.json"].Source != "vikunja" {
		t.Errorf("shared.json source = %s, want vikunja", byName["shared.json"].Source)
	}
	if len(got.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %+v", got.Conflicts)
	}
}

func TestMergeNoteItems_MultiIncludeDeclarationOrder(t *testing.T) {
	chain := []string{"root", "a", "b"}
	items := map[string][]types.Item{
		"root": {},
		"a":    {item("a1", "shared.md", "from-a")},
		"b":    {item("b1", "shared.md", "from-b")},
	}

	got := MergeNoteItems(chain, items)

	if len(got.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got.Items))
	}
	if got.Items[0].Item.ID != "a1" || got.Items[0].Source != "a" {
		t.Errorf("expected a to win over b, got %+v", got.Items[0])
	}
	if len(got.Conflicts) != 1 || got.Conflicts[0].Winner != "a" ||
		len(got.Conflicts[0].Losers) != 1 || got.Conflicts[0].Losers[0] != "b" {
		t.Errorf("expected winner=a losers=[b], got %+v", got.Conflicts)
	}
}

func TestMergeNoteItems_AggregatesMultipleConflicts(t *testing.T) {
	chain := []string{"root", "a", "b"}
	items := map[string][]types.Item{
		"root": {item("r1", "foo.md", "r")},
		"a":    {item("a1", "foo.md", "a"), item("a2", "bar.md", "a")},
		"b":    {item("b1", "foo.md", "b"), item("b2", "bar.md", "b")},
	}

	got := MergeNoteItems(chain, items)

	if len(got.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got.Items))
	}
	// Conflicts must be sorted by Name: bar.md then foo.md.
	if len(got.Conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d: %+v", len(got.Conflicts), got.Conflicts)
	}
	if got.Conflicts[0].Name != "bar.md" || got.Conflicts[0].Winner != "a" ||
		len(got.Conflicts[0].Losers) != 1 || got.Conflicts[0].Losers[0] != "b" {
		t.Errorf("bar.md conflict = %+v", got.Conflicts[0])
	}
	if got.Conflicts[1].Name != "foo.md" || got.Conflicts[1].Winner != "root" ||
		len(got.Conflicts[1].Losers) != 2 || got.Conflicts[1].Losers[0] != "a" || got.Conflicts[1].Losers[1] != "b" {
		t.Errorf("foo.md conflict = %+v; want winner=root losers=[a b] in chain order", got.Conflicts[1])
	}
}

func TestMergeNoteItems_CurrentFolderHasNoNotes(t *testing.T) {
	// Thin-project pattern: current folder has only pkv.include, all config
	// notes come from the included folder.
	chain := []string{"root", "vikunja"}
	items := map[string][]types.Item{
		"root":    {},
		"vikunja": {item("v1", "app.env.json", "v"), item("v2", "service.yaml", "y")},
	}

	got := MergeNoteItems(chain, items)

	if len(got.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got.Items))
	}
	for _, it := range got.Items {
		if it.Source != "vikunja" {
			t.Errorf("item %s source=%s, want vikunja", it.Item.Name, it.Source)
		}
	}
	if len(got.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %+v", got.Conflicts)
	}
}

func TestMergeNoteItems_AllFoldersEmpty(t *testing.T) {
	chain := []string{"root", "a"}
	items := map[string][]types.Item{"root": {}, "a": {}}

	got := MergeNoteItems(chain, items)

	if len(got.Items) != 0 {
		t.Errorf("expected no items, got %d", len(got.Items))
	}
	if len(got.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %+v", got.Conflicts)
	}
	if len(got.Chain) != 2 || got.Chain[0] != "root" || got.Chain[1] != "a" {
		t.Errorf("chain not preserved: %+v", got.Chain)
	}
}

func TestMergeNoteItems_ReservedMetadataDropped(t *testing.T) {
	// Even if a caller forgets to filter, merge should never leak
	// pkv.env / pkv.include into the merged config set.
	chain := []string{"root"}
	items := map[string][]types.Item{
		"root": {
			item("1", types.ReservedIncludeNoteName, "vikunja\n"),
			item("2", types.ReservedEnvNoteName, "FOO=1"),
			item("3", "legit.md", "x"),
		},
	}

	got := MergeNoteItems(chain, items)

	if len(got.Items) != 1 {
		t.Fatalf("expected 1 item (legit.md only), got %d: %+v", len(got.Items), got.Items)
	}
	if got.Items[0].Item.Name != "legit.md" {
		t.Errorf("expected legit.md, got %s", got.Items[0].Item.Name)
	}
}

func TestMergeNoteItems_EmptyChain(t *testing.T) {
	got := MergeNoteItems(nil, nil)
	if len(got.Items) != 0 || len(got.Conflicts) != 0 {
		t.Errorf("expected empty merged result, got %+v", got)
	}
	if len(got.Chain) != 0 {
		t.Errorf("expected empty chain, got %+v", got.Chain)
	}
}
