package env

import (
	"reflect"
	"strings"
	"testing"
)

func TestMergePkvEnvNotes_SingleFolderNoInclude(t *testing.T) {
	// With no include chain the result should match the existing single-folder
	// behavior: every var in the body, sorted by key, all sourced to the
	// current folder, no conflicts.
	res, err := MergePkvEnvNotes([]string{"root"}, map[string]string{
		"root": "FOO=1\nBAR=2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []SourcedEnvVar{
		{EnvVar: EnvVar{Key: "BAR", Value: "2"}, Source: "root"},
		{EnvVar: EnvVar{Key: "FOO", Value: "1"}, Source: "root"},
	}
	if !reflect.DeepEqual(res.Vars, want) {
		t.Errorf("Vars mismatch\n  got:  %+v\n  want: %+v", res.Vars, want)
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %+v", res.Conflicts)
	}
	if !reflect.DeepEqual(res.Chain, []string{"root"}) {
		t.Errorf("Chain mismatch: got %v want [root]", res.Chain)
	}
}

func TestMergePkvEnvNotes_CurrentFolderWins(t *testing.T) {
	// When current folder and include both define FOO, current wins.
	res, err := MergePkvEnvNotes(
		[]string{"root", "vikunja"},
		map[string]string{
			"root":    "FOO=current",
			"vikunja": "FOO=shared",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Vars) != 1 || res.Vars[0].Key != "FOO" || res.Vars[0].Value != "current" || res.Vars[0].Source != "root" {
		t.Errorf("expected FOO=current from root, got %+v", res.Vars)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("expected one conflict, got %+v", res.Conflicts)
	}
	c := res.Conflicts[0]
	if c.Key != "FOO" || c.Winner != "root" || !reflect.DeepEqual(c.Losers, []string{"vikunja"}) {
		t.Errorf("conflict mismatch: %+v", c)
	}
}

func TestMergePkvEnvNotes_IncludeProvidesDefault(t *testing.T) {
	// When only the include defines BAR, it should be picked up and sourced to
	// the include folder. No conflict because only one folder declares it.
	res, err := MergePkvEnvNotes(
		[]string{"root", "vikunja"},
		map[string]string{
			"root":    "FOO=1",
			"vikunja": "BAR=2",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []SourcedEnvVar{
		{EnvVar: EnvVar{Key: "BAR", Value: "2"}, Source: "vikunja"},
		{EnvVar: EnvVar{Key: "FOO", Value: "1"}, Source: "root"},
	}
	if !reflect.DeepEqual(res.Vars, want) {
		t.Errorf("Vars mismatch\n  got:  %+v\n  want: %+v", res.Vars, want)
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %+v", res.Conflicts)
	}
}

func TestMergePkvEnvNotes_MultiIncludeDeclarationOrder(t *testing.T) {
	// chain is [root, a, b]; root has no FOO, a has FOO=a, b has FOO=b.
	// a should win because it appears first on the chain (chain is resolver
	// output, which preserves pkv.include declaration order).
	res, err := MergePkvEnvNotes(
		[]string{"root", "a", "b"},
		map[string]string{
			"a": "FOO=a",
			"b": "FOO=b\nBAZ=fromb",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var foo *SourcedEnvVar
	var baz *SourcedEnvVar
	for i := range res.Vars {
		switch res.Vars[i].Key {
		case "FOO":
			foo = &res.Vars[i]
		case "BAZ":
			baz = &res.Vars[i]
		}
	}
	if foo == nil || foo.Value != "a" || foo.Source != "a" {
		t.Errorf("expected FOO=a from a, got %+v", foo)
	}
	if baz == nil || baz.Value != "fromb" || baz.Source != "b" {
		t.Errorf("expected BAZ=fromb from b, got %+v", baz)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0].Key != "FOO" || res.Conflicts[0].Winner != "a" ||
		!reflect.DeepEqual(res.Conflicts[0].Losers, []string{"b"}) {
		t.Errorf("conflict mismatch: %+v", res.Conflicts)
	}
}

func TestMergePkvEnvNotes_AggregatesMultipleConflicts(t *testing.T) {
	// Two keys collide across three folders. Check the conflict list records
	// all losers in chain order and is sorted by key.
	res, err := MergePkvEnvNotes(
		[]string{"root", "a", "b"},
		map[string]string{
			"root": "FOO=1\nBAR=1",
			"a":    "FOO=a\nBAR=a",
			"b":    "FOO=b",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %+v", res.Conflicts)
	}
	// sorted: BAR then FOO
	if res.Conflicts[0].Key != "BAR" || res.Conflicts[0].Winner != "root" ||
		!reflect.DeepEqual(res.Conflicts[0].Losers, []string{"a"}) {
		t.Errorf("BAR conflict mismatch: %+v", res.Conflicts[0])
	}
	if res.Conflicts[1].Key != "FOO" || res.Conflicts[1].Winner != "root" ||
		!reflect.DeepEqual(res.Conflicts[1].Losers, []string{"a", "b"}) {
		t.Errorf("FOO conflict mismatch: %+v", res.Conflicts[1])
	}
}

func TestMergePkvEnvNotes_CurrentFolderHasNoEnvNote(t *testing.T) {
	// Current folder declares no pkv.env; include provides the full set. This
	// mirrors the "thin project" pattern: pkv.include + a shared folder.
	res, err := MergePkvEnvNotes(
		[]string{"root", "vikunja"},
		map[string]string{
			"vikunja": "VIKUNJA_URL=https://v.example\nVIKUNJA_API_TOKEN=secret",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Vars) != 2 {
		t.Fatalf("expected 2 vars, got %+v", res.Vars)
	}
	for _, v := range res.Vars {
		if v.Source != "vikunja" {
			t.Errorf("expected source=vikunja, got %+v", v)
		}
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %+v", res.Conflicts)
	}
}

func TestMergePkvEnvNotes_AllFoldersEmpty(t *testing.T) {
	// No folder on the chain has pkv.env. Result must be empty but not nil
	// in shape and Chain must still be populated so callers can log it.
	res, err := MergePkvEnvNotes([]string{"root", "a"}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Vars) != 0 {
		t.Errorf("expected empty Vars, got %+v", res.Vars)
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("expected empty Conflicts, got %+v", res.Conflicts)
	}
	if !reflect.DeepEqual(res.Chain, []string{"root", "a"}) {
		t.Errorf("Chain mismatch: %v", res.Chain)
	}
}

func TestMergePkvEnvNotes_ParseErrorFromIncludeFolder(t *testing.T) {
	// A bad body in an include folder must surface the folder name so the
	// user can go fix the right pkv.env.
	_, err := MergePkvEnvNotes(
		[]string{"root", "vikunja"},
		map[string]string{
			"root":    "FOO=1",
			"vikunja": "this-is-not-kv",
		},
	)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), `folder "vikunja"`) {
		t.Errorf("error should mention bad folder; got %v", err)
	}
}

func TestMergePkvEnvNotes_EmptyChainIsError(t *testing.T) {
	_, err := MergePkvEnvNotes(nil, map[string]string{})
	if err == nil {
		t.Fatal("expected error for empty chain, got nil")
	}
}

func TestMergePkvEnvNotes_FolderWithEmptyBodyIsSkipped(t *testing.T) {
	// An explicit "" body (e.g. a pkv.env note existing but blank) must not
	// crash and must not contribute vars.
	res, err := MergePkvEnvNotes(
		[]string{"root", "a"},
		map[string]string{
			"root": "",
			"a":    "FOO=1",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Vars) != 1 || res.Vars[0].Source != "a" {
		t.Errorf("expected single FOO from a, got %+v", res.Vars)
	}
}
