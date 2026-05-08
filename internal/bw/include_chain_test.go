package bw

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
)

// fakeVault is a minimal in-memory Bitwarden stand-in for LoadIncludeChain
// tests. It tracks call counts so tests can assert the cache-once contract.
type fakeVault struct {
	folders []types.Folder
	// itemsByFolderID maps folder ID -> items (typically a single pkv.include).
	itemsByFolderID map[string][]types.Item

	foldersErr error
	itemsErr   map[string]error

	listFoldersCalls int
	listItemsCalls   map[string]int
}

func newFakeVault() *fakeVault {
	return &fakeVault{
		itemsByFolderID: map[string][]types.Item{},
		itemsErr:        map[string]error{},
		listItemsCalls:  map[string]int{},
	}
}

func (f *fakeVault) addFolder(id, name string) {
	f.folders = append(f.folders, types.Folder{ID: id, Name: name})
}

// setIncludeNote installs a single pkv.include Secure Note with the given body.
func (f *fakeVault) setIncludeNote(folderID, body string) {
	f.itemsByFolderID[folderID] = append(f.itemsByFolderID[folderID], types.Item{
		ID:       folderID + "-include",
		FolderID: folderID,
		Type:     types.ItemTypeSecureNote,
		Name:     types.ReservedIncludeNoteName,
		Notes:    body,
	})
}

func (f *fakeVault) listFolders() ([]types.Folder, error) {
	f.listFoldersCalls++
	if f.foldersErr != nil {
		return nil, f.foldersErr
	}
	return append([]types.Folder(nil), f.folders...), nil
}

func (f *fakeVault) listItems(folderID string) ([]types.Item, error) {
	f.listItemsCalls[folderID]++
	if err := f.itemsErr[folderID]; err != nil {
		return nil, err
	}
	return append([]types.Item(nil), f.itemsByFolderID[folderID]...), nil
}

func foldersToNames(folders []types.Folder) []string {
	out := make([]string, 0, len(folders))
	for _, f := range folders {
		out = append(out, f.Name)
	}
	return out
}

func TestLoadIncludeChain_SimpleChainPreservesOrder(t *testing.T) {
	// root -> (a, b); a -> c
	v := newFakeVault()
	v.addFolder("root-id", "root")
	v.addFolder("a-id", "a")
	v.addFolder("b-id", "b")
	v.addFolder("c-id", "c")
	v.setIncludeNote("root-id", "a\nb\n")
	v.setIncludeNote("a-id", "c\n")

	got, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantNames := []string{"root", "a", "c", "b"}
	if !reflect.DeepEqual(foldersToNames(got), wantNames) {
		t.Errorf("chain names = %v, want %v", foldersToNames(got), wantNames)
	}
	// IDs should be populated from the folder lookup.
	wantIDs := map[string]string{"root": "root-id", "a": "a-id", "b": "b-id", "c": "c-id"}
	for _, f := range got {
		if wantIDs[f.Name] != f.ID {
			t.Errorf("folder %q ID = %q, want %q", f.Name, f.ID, wantIDs[f.Name])
		}
	}
}

func TestLoadIncludeChain_CachesFolderList(t *testing.T) {
	v := newFakeVault()
	v.addFolder("r", "root")
	v.addFolder("a", "a")
	v.addFolder("b", "b")
	v.setIncludeNote("r", "a\nb\n")
	// a and b are leaves with no pkv.include note.

	_, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.listFoldersCalls != 1 {
		t.Errorf("ListFolders called %d times, want 1 (cache-once contract)", v.listFoldersCalls)
	}
	// Each visited folder should be queried at most once for items.
	for id, n := range v.listItemsCalls {
		if n > 1 {
			t.Errorf("ListItems(%q) called %d times, want <= 1 per visit", id, n)
		}
	}
}

func TestLoadIncludeChain_RootWithNoIncludeNote(t *testing.T) {
	// Root is in the vault but has no pkv.include; chain is just [root].
	v := newFakeVault()
	v.addFolder("r", "root")

	got, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(foldersToNames(got), []string{"root"}) {
		t.Errorf("chain = %v, want [root]", foldersToNames(got))
	}
}

func TestLoadIncludeChain_MissingRootIsReported(t *testing.T) {
	v := newFakeVault()
	// Root not present.
	v.addFolder("a", "a")

	_, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err == nil {
		t.Fatal("expected error for missing root")
	}
	if !strings.Contains(err.Error(), "missing folders: root") {
		t.Errorf("error = %v, want to mention missing root", err)
	}
}

func TestLoadIncludeChain_AggregatesMissingIncludes(t *testing.T) {
	// root references two folders; neither exists. Both should be reported.
	v := newFakeVault()
	v.addFolder("r", "root")
	v.setIncludeNote("r", "alpha\nbeta\n")

	_, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err == nil {
		t.Fatal("expected aggregated missing-folder error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "alpha") || !strings.Contains(msg, "beta") {
		t.Errorf("error = %v, should mention both missing folders alpha and beta", err)
	}
	// Sanity: the message should label them as a single issue batch.
	if !strings.Contains(msg, "missing folders") {
		t.Errorf("error = %v, should label the issue batch", err)
	}
}

func TestLoadIncludeChain_AggregatesDuplicateFolderNames(t *testing.T) {
	// Vault contains two folders with the same name. Referencing `a` must not
	// silently pick one; it must be reported as a duplicate.
	v := newFakeVault()
	v.addFolder("r", "root")
	v.addFolder("a1", "a")
	v.addFolder("a2", "a")
	v.setIncludeNote("r", "a\n")

	_, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err == nil {
		t.Fatal("expected duplicate-name error")
	}
	if !strings.Contains(err.Error(), "duplicate folder names") {
		t.Errorf("error = %v, should flag duplicate folder names", err)
	}
	if !strings.Contains(err.Error(), "a") {
		t.Errorf("error = %v, should name the duplicate", err)
	}
}

func TestLoadIncludeChain_AggregatesMultiplePkvIncludeNotes(t *testing.T) {
	// A single folder contains two pkv.include notes → misconfiguration.
	v := newFakeVault()
	v.addFolder("r", "root")
	v.addFolder("dupnotes", "dupnotes")
	v.setIncludeNote("r", "dupnotes\n")
	// Install a second pkv.include in dupnotes on top of the first.
	v.setIncludeNote("dupnotes", "anything")
	v.setIncludeNote("dupnotes", "another")

	_, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err == nil {
		t.Fatal("expected multiple-pkv.include error")
	}
	if !strings.Contains(err.Error(), "dupnotes") {
		t.Errorf("error = %v, should name the offending folder", err)
	}
}

func TestLoadIncludeChain_AggregatesAcrossCategories(t *testing.T) {
	// Mix of issues: one missing include and one duplicate folder name.
	// Both should surface in the same error message.
	v := newFakeVault()
	v.addFolder("r", "root")
	v.addFolder("dup1", "dup")
	v.addFolder("dup2", "dup")
	v.setIncludeNote("r", "dup\nmissing\n")

	_, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing folders: missing") {
		t.Errorf("error = %v, should list missing folder", err)
	}
	if !strings.Contains(msg, "duplicate folder names") {
		t.Errorf("error = %v, should list duplicate names", err)
	}
}

func TestLoadIncludeChain_CyclePropagates(t *testing.T) {
	// root -> a -> root (cycle)
	v := newFakeVault()
	v.addFolder("r", "root")
	v.addFolder("a", "a")
	v.setIncludeNote("r", "a\n")
	v.setIncludeNote("a", "root\n")

	_, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %v, should surface cycle from resolver", err)
	}
}

func TestLoadIncludeChain_ListFoldersError(t *testing.T) {
	v := newFakeVault()
	v.foldersErr = errors.New("vault network failure")

	_, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err == nil {
		t.Fatal("expected error from ListFolders")
	}
	if !errors.Is(err, v.foldersErr) {
		t.Errorf("error = %v, should wrap original", err)
	}
}

func TestLoadIncludeChain_ListItemsErrorOnPath(t *testing.T) {
	v := newFakeVault()
	v.addFolder("r", "root")
	v.addFolder("a", "a")
	v.setIncludeNote("r", "a\n")
	v.itemsErr["a"] = errors.New("item fetch failed")

	_, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err == nil {
		t.Fatal("expected error from ListItems")
	}
	if !errors.Is(err, v.itemsErr["a"]) {
		t.Errorf("error = %v, should wrap the underlying fetch error", err)
	}
}

func TestLoadIncludeChain_EmptyRootRejected(t *testing.T) {
	v := newFakeVault()
	_, err := loadIncludeChain("", v.listFolders, v.listItems)
	if err == nil {
		t.Fatal("expected error for empty root")
	}
	// Should not even call the vault.
	if v.listFoldersCalls != 0 {
		t.Errorf("ListFolders called %d times, want 0 for empty root", v.listFoldersCalls)
	}
}

func TestLoadIncludeChain_DiamondVisitsEachFolderOnce(t *testing.T) {
	// root -> (a, b); a -> shared; b -> shared.
	// shared must be visited only once by ListItems (dedup in resolver).
	v := newFakeVault()
	v.addFolder("r", "root")
	v.addFolder("a", "a")
	v.addFolder("b", "b")
	v.addFolder("s", "shared")
	v.setIncludeNote("r", "a\nb\n")
	v.setIncludeNote("a", "shared\n")
	v.setIncludeNote("b", "shared\n")

	got, err := loadIncludeChain("root", v.listFolders, v.listItems)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantNames := []string{"root", "a", "shared", "b"}
	if !reflect.DeepEqual(foldersToNames(got), wantNames) {
		t.Errorf("chain = %v, want %v", foldersToNames(got), wantNames)
	}
	if v.listItemsCalls["s"] != 1 {
		t.Errorf("ListItems(shared) = %d, want 1 (resolver must dedup)", v.listItemsCalls["s"])
	}
}

// TestLoadIncludeChain_RootCaseInsensitive: vault stores "Dec" but the user
// passes "dec". loadIncludeChain must fall back to a case-insensitive match
// and report the chain using the vault canonical name.
func TestLoadIncludeChain_RootCaseInsensitive(t *testing.T) {
	v := newFakeVault()
	v.addFolder("dec-id", "Dec")

	got, err := loadIncludeChain("dec", v.listFolders, v.listItems)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if names := foldersToNames(got); !reflect.DeepEqual(names, []string{"Dec"}) {
		t.Errorf("chain names = %v, want [Dec] (canonical from vault)", names)
	}
	if got[0].ID != "dec-id" {
		t.Errorf("chain[0].ID = %q, want %q", got[0].ID, "dec-id")
	}
}

// TestLoadIncludeChain_IncludeBodyCaseInsensitive: include body lists "DEC"
// while vault has "dec". The walker must normalize each include line via
// case-insensitive fallback and use vault canonical names in the chain.
func TestLoadIncludeChain_IncludeBodyCaseInsensitive(t *testing.T) {
	v := newFakeVault()
	v.addFolder("app-id", "app")
	v.addFolder("dec-id", "dec")
	v.setIncludeNote("app-id", "DEC\n")

	got, err := loadIncludeChain("app", v.listFolders, v.listItems)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if names := foldersToNames(got); !reflect.DeepEqual(names, []string{"app", "dec"}) {
		t.Errorf("chain names = %v, want [app dec]", names)
	}
}

// TestLoadIncludeChain_ExactPreferredOverCaseInsensitive: vault has both
// "Dec" and "dec"; an exact-case query must win without triggering the
// case-insensitive ambiguity path.
func TestLoadIncludeChain_ExactPreferredOverCaseInsensitive(t *testing.T) {
	v := newFakeVault()
	v.addFolder("dec-cap-id", "Dec")
	v.addFolder("dec-low-id", "dec")

	got, err := loadIncludeChain("dec", v.listFolders, v.listItems)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ID != "dec-low-id" || got[0].Name != "dec" {
		t.Errorf("chain[0] = %+v, want exact match on lowercase 'dec'", got[0])
	}
}

// TestLoadIncludeChain_AmbiguousCaseInsensitive: input has no exact match
// and case-insensitive lookup yields multiple candidates. Must abort with
// an aggregated error naming both candidates.
func TestLoadIncludeChain_AmbiguousCaseInsensitive(t *testing.T) {
	v := newFakeVault()
	v.addFolder("dec-cap-id", "Dec")
	v.addFolder("dec-up-id", "DEC")

	_, err := loadIncludeChain("dec", v.listFolders, v.listItems)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, `ambiguous folder name "dec"`) {
		t.Errorf("error = %q, want substring about ambiguous folder", msg)
	}
	if !strings.Contains(msg, "DEC") || !strings.Contains(msg, "Dec") {
		t.Errorf("error = %q, should list both candidates DEC and Dec", msg)
	}
}
