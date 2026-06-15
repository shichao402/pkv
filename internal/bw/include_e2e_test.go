package bw

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/env"
	"github.com/shichao402/pkv/internal/note"
	"github.com/shichao402/pkv/internal/state"
)

// The tests in this file exercise the full pkv.include MVP contract against
// an in-memory vault. They drive the same package-private seam
// (loadIncludeChain) that production cmd-layer callers use via
// Client.LoadIncludeChain, then feed the resolved chain into the real
// internal/env, internal/note, and internal/state pipelines. HOME is
// redirected to t.TempDir() so artifact and state writes land in an
// isolated location.
//
// This is deliberately broader than the focused unit tests already in this
// package (include_chain_test.go): those verify chain construction; these
// verify that chain construction plus env merge/deploy plus note
// merge/sync plus state recording compose correctly end-to-end.

// fakeVaultAddItem appends a non-include item (pkv.env or a config note) to
// the vault. The existing fakeVault.setIncludeNote helper is intentionally
// scoped to pkv.include only; this helper lets tests attach arbitrary
// Secure Notes without mutating the original fixture.
func fakeVaultAddItem(v *fakeVault, folderID, itemID, name, body string) {
	v.itemsByFolderID[folderID] = append(v.itemsByFolderID[folderID], types.Item{
		ID:       itemID,
		FolderID: folderID,
		Type:     types.ItemTypeSecureNote,
		Name:     name,
		Notes:    body,
	})
}

// runIncludePipeline is the smallest wrapper that mirrors what
// cmd/resources.go does for `pkv get <folder> env` + `pkv get <folder> note`,
// minus the Cobra glue and the bw.Client construction. It loads the include
// chain, runs env merge+deploy, runs note merge+sync, and persists state.
//
// Returns any error from the chain load or the env/note pipeline so tests
// can assert both success and failure modes without reimplementing the
// orchestration.
func runIncludePipeline(
	t *testing.T,
	v *fakeVault,
	root string,
	targetDir string,
) (envResult env.MergeResult, noteResult note.MergedNotes, err error) {
	t.Helper()

	chain, chainErr := loadIncludeChain(root, v.listFolders, v.listItems)
	if chainErr != nil {
		return env.MergeResult{}, note.MergedNotes{}, chainErr
	}

	// Gather per-folder items so the chain can be fed to the env + note
	// merge functions. This mirrors cmd/resources.go's second pass over the
	// chain.
	chainNames := make([]string, 0, len(chain))
	envBodies := make(map[string]string, len(chain))
	configNotesByFolder := make(map[string][]types.Item, len(chain))
	var currentEnvItem types.Item
	var hasCurrentEnv bool

	for i, f := range chain {
		chainNames = append(chainNames, f.Name)
		items, listErr := v.listItems(f.ID)
		if listErr != nil {
			return env.MergeResult{}, note.MergedNotes{}, listErr
		}

		if envItem, ok, envErr := FindManagedEnvNote(items); envErr != nil {
			return env.MergeResult{}, note.MergedNotes{}, envErr
		} else if ok {
			envBodies[f.Name] = envItem.Notes
			if i == 0 {
				currentEnvItem = envItem
				hasCurrentEnv = true
			}
		}

		configNotesByFolder[f.Name] = FilterConfigNotes(items)
	}

	mergedEnv, mergeErr := env.MergePkvEnvNotes(chainNames, envBodies)
	if mergeErr != nil {
		return env.MergeResult{}, note.MergedNotes{}, mergeErr
	}

	st, stErr := state.Load()
	if stErr != nil {
		return env.MergeResult{}, note.MergedNotes{}, stErr
	}

	// Only deploy env artifacts when there's something to write. This
	// matches the guard in cmd/resources.go's getEnvCmd: a chain with no
	// pkv.env anywhere shouldn't produce an env artifact.
	if len(mergedEnv.Vars) > 0 {
		deployer := env.NewDeployer(st)
		if _, deployErr := deployer.DeployMerged(root, mergedEnv, currentEnvItem, hasCurrentEnv); deployErr != nil {
			return env.MergeResult{}, note.MergedNotes{}, deployErr
		}
	}

	mergedNotes := note.MergeNoteItems(chainNames, configNotesByFolder)

	if len(mergedNotes.Items) > 0 {
		sourceByID := make(map[string]string, len(mergedNotes.Items))
		syncItems := make([]types.Item, 0, len(mergedNotes.Items))
		for i := range mergedNotes.Items {
			sn := mergedNotes.Items[i]
			syncItems = append(syncItems, sn.Item)
			sourceByID[sn.Item.ID] = sn.Source
		}
		syncer := note.NewSyncer(st)
		if _, syncErr := syncer.SyncFolderWithSources(syncItems, sourceByID, targetDir, root); syncErr != nil {
			return env.MergeResult{}, note.MergedNotes{}, syncErr
		}
	}

	if saveErr := st.Save(); saveErr != nil {
		return env.MergeResult{}, note.MergedNotes{}, saveErr
	}

	return mergedEnv, mergedNotes, nil
}

// readEnvArtifactJSON reads the JSON env artifact that DeployMerged writes
// under $HOME/.pkv/env/<folder>.json, returning the parsed key/value map.
func readEnvArtifactJSON(t *testing.T, folder string) map[string]string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	path := filepath.Join(home, ".pkv", "env", folder+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env artifact %q: %v", path, err)
	}
	got := make(map[string]string)
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal env artifact: %v", err)
	}
	return got
}

// TestLoadIncludeChain_E2E_HappyPath covers scenario 1 from the plan:
// two folders each carrying pkv.env + one config note; A includes B.
// Asserts env artifact merge, note files on disk, state SourceFolder
// attribution, and that pkv.include itself is never synced as a config
// file.
func TestLoadIncludeChain_E2E_HappyPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	notesDir := t.TempDir()

	v := newFakeVault()
	v.addFolder("a-id", "A")
	v.addFolder("b-id", "B")
	v.setIncludeNote("a-id", "B\n")
	fakeVaultAddItem(v, "a-id", "a-env", types.ReservedEnvNoteName, "KEY_A=a_value\n")
	fakeVaultAddItem(v, "a-id", "a-cfg1", "app.json", `{"a":1}`)
	fakeVaultAddItem(v, "b-id", "b-env", types.ReservedEnvNoteName, "KEY_B=b_value\n")
	fakeVaultAddItem(v, "b-id", "b-cfg1", "extra.json", `{"shared":true}`)

	mergedEnv, mergedNotes, err := runIncludePipeline(t, v, "A", notesDir)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	// Env merge: both keys present, no conflicts.
	gotEnv := readEnvArtifactJSON(t, "A")
	if gotEnv["KEY_A"] != "a_value" || gotEnv["KEY_B"] != "b_value" {
		t.Errorf("env artifact = %v, want KEY_A=a_value KEY_B=b_value", gotEnv)
	}
	if len(mergedEnv.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %+v", mergedEnv.Conflicts)
	}

	// Note files: both config notes written, pkv.include NOT written.
	for _, rel := range []string{"app.json", "extra.json"} {
		p := filepath.Join(notesDir, rel)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected note %q on disk: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(notesDir, types.ReservedIncludeNoteName)); err == nil {
		t.Errorf("pkv.include was written to disk; it must be metadata-only")
	}

	// Merge bookkeeping should know about both synced notes.
	if len(mergedNotes.Items) != 2 {
		t.Fatalf("mergedNotes.Items len = %d, want 2", len(mergedNotes.Items))
	}

	// State attribution: env owned by A (SourceFolder empty); B's note
	// carries SourceFolder="B".
	st, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	envs := st.FindEnvsByFolder("A")
	if len(envs) != 1 {
		t.Fatalf("expected 1 env entry for A, got %d", len(envs))
	}
	if envs[0].SourceFolder != "" {
		t.Errorf("A owns its own pkv.env; SourceFolder should be empty, got %q", envs[0].SourceFolder)
	}
	if envs[0].ItemID != "a-env" {
		t.Errorf("env entry ItemID = %q, want a-env", envs[0].ItemID)
	}

	notes := st.FindSyncedNotesByFolder("A")
	if len(notes) != 2 {
		t.Fatalf("expected 2 note entries for A, got %d", len(notes))
	}
	gotSources := map[string]string{}
	for _, n := range notes {
		gotSources[n.FileName] = n.SourceFolder
	}
	if gotSources["app.json"] != "" {
		t.Errorf("app.json SourceFolder = %q, want empty (owned by A)", gotSources["app.json"])
	}
	if gotSources["extra.json"] != "B" {
		t.Errorf("extra.json SourceFolder = %q, want B", gotSources["extra.json"])
	}
}

// TestLoadIncludeChain_E2E_ConflictPreflightCurrentWins covers scenario 3:
// P and Q both declare KEY_DB; P includes Q. Verifies current-folder-wins
// is non-fatal and the conflict is reported in the merge result.
func TestLoadIncludeChain_E2E_ConflictPreflightCurrentWins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	notesDir := t.TempDir()

	v := newFakeVault()
	v.addFolder("p-id", "P")
	v.addFolder("q-id", "Q")
	v.setIncludeNote("p-id", "Q\n")
	fakeVaultAddItem(v, "p-id", "p-env", types.ReservedEnvNoteName, "KEY_DB=prod\nKEY_LOG=prod-only\n")
	fakeVaultAddItem(v, "q-id", "q-env", types.ReservedEnvNoteName, "KEY_DB=qa\n")

	mergedEnv, _, err := runIncludePipeline(t, v, "P", notesDir)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	gotEnv := readEnvArtifactJSON(t, "P")
	if gotEnv["KEY_DB"] != "prod" {
		t.Errorf("KEY_DB = %q, want prod (P wins)", gotEnv["KEY_DB"])
	}
	if gotEnv["KEY_LOG"] != "prod-only" {
		t.Errorf("KEY_LOG = %q, want prod-only", gotEnv["KEY_LOG"])
	}

	if len(mergedEnv.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(mergedEnv.Conflicts), mergedEnv.Conflicts)
	}
	c := mergedEnv.Conflicts[0]
	if c.Key != "KEY_DB" || c.Winner != "P" || len(c.Losers) != 1 || c.Losers[0] != "Q" {
		t.Errorf("conflict = %+v, want {Key:KEY_DB Winner:P Losers:[Q]}", c)
	}
}

// TestLoadIncludeChain_E2E_CycleHardFails covers scenario 4: R -> S -> R
// must be rejected by loadIncludeChain and must not produce any artifact
// or state mutation.
func TestLoadIncludeChain_E2E_CycleHardFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notesDir := t.TempDir()

	v := newFakeVault()
	v.addFolder("r-id", "R")
	v.addFolder("s-id", "S")
	v.setIncludeNote("r-id", "S\n")
	v.setIncludeNote("s-id", "R\n")

	_, _, err := runIncludePipeline(t, v, "R", notesDir)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("error %q does not mention cycle", err.Error())
	}

	// Nothing should have been written to $HOME/.pkv.
	if _, statErr := os.Stat(filepath.Join(home, ".pkv")); statErr == nil {
		t.Errorf("$HOME/.pkv was created despite cycle failure")
	}

	// And nothing in the notes target dir.
	entries, readErr := os.ReadDir(notesDir)
	if readErr != nil {
		t.Fatalf("read notesDir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("notesDir should be empty after cycle failure, got %d entries", len(entries))
	}
}

// TestLoadIncludeChain_E2E_AggregatedPreflightErrors covers scenario 7:
// missing folders + a duplicate-named folder should aggregate into a
// single error with all issues listed, and nothing written.
func TestLoadIncludeChain_E2E_AggregatedPreflightErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notesDir := t.TempDir()

	v := newFakeVault()
	v.addFolder("u-id", "U")
	// Two folders share the name "dup" -> loadIncludeChain reports duplicate.
	v.addFolder("dup-id-1", "dup")
	v.addFolder("dup-id-2", "dup")
	v.setIncludeNote("u-id", "missing_x\ndup\nmissing_z\n")

	_, _, err := runIncludePipeline(t, v, "U", notesDir)
	if err == nil {
		t.Fatal("expected aggregated preflight error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"missing_x", "missing_z", "dup", "duplicate folder names"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing substring %q", msg, want)
		}
	}
	// Missing folders block should list both names together.
	if !strings.Contains(msg, "missing folders: missing_x, missing_z") {
		t.Errorf("error %q should aggregate missing folders in a single line", msg)
	}

	if _, statErr := os.Stat(filepath.Join(home, ".pkv")); statErr == nil {
		t.Errorf("$HOME/.pkv was created despite preflight failure")
	}
	entries, readErr := os.ReadDir(notesDir)
	if readErr != nil {
		t.Fatalf("read notesDir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("notesDir should be empty after preflight failure, got %d entries", len(entries))
	}
}
