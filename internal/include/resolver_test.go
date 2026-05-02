package include

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
)

func TestParseLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "whitespace only",
			input: "   \n\t\n   ",
			want:  nil,
		},
		{
			name:  "simple list",
			input: "vikunja\npub_res\n",
			want:  []string{"vikunja", "pub_res"},
		},
		{
			name:  "trims surrounding whitespace",
			input: "  vikunja  \n\t pub_res \n",
			want:  []string{"vikunja", "pub_res"},
		},
		{
			name:  "skips blank lines",
			input: "vikunja\n\n\npub_res\n",
			want:  []string{"vikunja", "pub_res"},
		},
		{
			name:  "skips comment lines",
			input: "# top level include\nvikunja\n# another comment\npub_res\n",
			want:  []string{"vikunja", "pub_res"},
		},
		{
			name:  "comment with leading whitespace",
			input: "   # indented comment\nvikunja\n",
			want:  []string{"vikunja"},
		},
		{
			name:  "preserves intra-body duplicates",
			input: "a\nb\na\n",
			want:  []string{"a", "b", "a"},
		},
		{
			name:  "trailing newline is fine",
			input: "vikunja",
			want:  []string{"vikunja"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLines(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseLines(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindIncludeNote(t *testing.T) {
	tests := []struct {
		name      string
		items     []types.Item
		wantFound bool
		wantErr   bool
		wantID    string
	}{
		{
			name:      "no items",
			items:     nil,
			wantFound: false,
		},
		{
			name: "no pkv.include present",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSecureNote, Name: "other"},
				{ID: "2", Type: types.ItemTypeLogin, Name: types.ReservedIncludeNoteName},
			},
			wantFound: false,
		},
		{
			name: "single pkv.include secure note",
			items: []types.Item{
				{ID: "abc", Type: types.ItemTypeSecureNote, Name: types.ReservedIncludeNoteName, Notes: "vikunja"},
				{ID: "xyz", Type: types.ItemTypeSecureNote, Name: "readme"},
			},
			wantFound: true,
			wantID:    "abc",
		},
		{
			name: "ignores non-secure-note with reserved name",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeLogin, Name: types.ReservedIncludeNoteName},
			},
			wantFound: false,
		},
		{
			name: "multiple matches is an error",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSecureNote, Name: types.ReservedIncludeNoteName},
				{ID: "2", Type: types.ItemTypeSecureNote, Name: types.ReservedIncludeNoteName},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found, err := FindIncludeNote(tt.items)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if found && got.ID != tt.wantID {
				t.Errorf("got ID %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

// fetcherFromMap builds a FetchFunc that returns the body for folders present
// in the map. Folders not in the map are reported as non-existent (leaf).
func fetcherFromMap(m map[string]string) FetchFunc {
	return func(folder string) (string, bool, error) {
		body, ok := m[folder]
		if !ok {
			return "", false, nil
		}
		return body, true, nil
	}
}

func TestResolve_EmptyPkvInclude(t *testing.T) {
	// pkv.include exists but body is empty/whitespace/comments only → treated
	// as "no includes", not an error.
	cases := []string{
		"",
		"   ",
		"# just a comment\n",
		"\n\n  \n",
	}
	for _, body := range cases {
		chain, err := Resolve("root", fetcherFromMap(map[string]string{"root": body}))
		if err != nil {
			t.Errorf("body %q: unexpected error %v", body, err)
		}
		if !reflect.DeepEqual(chain, []string{"root"}) {
			t.Errorf("body %q: chain = %v, want [root]", body, chain)
		}
	}
}

func TestResolve_NoIncludeNote(t *testing.T) {
	// Root folder has no pkv.include note at all → chain is [root].
	chain, err := Resolve("root", fetcherFromMap(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(chain, []string{"root"}) {
		t.Errorf("chain = %v, want [root]", chain)
	}
}

func TestResolve_NormalChain(t *testing.T) {
	// root -> (a, b); a -> (c); b -> (); c -> ().
	// Expected DFS order: root, a, c, b.
	fetch := fetcherFromMap(map[string]string{
		"root": "a\nb\n",
		"a":    "c\n",
		"b":    "",
		"c":    "",
	})
	chain, err := Resolve("root", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"root", "a", "c", "b"}
	if !reflect.DeepEqual(chain, want) {
		t.Errorf("chain = %v, want %v", chain, want)
	}
}

func TestResolve_CommentsAndBlanksInBody(t *testing.T) {
	fetch := fetcherFromMap(map[string]string{
		"root": "# include the shared vikunja config\nvikunja\n\n# next\npub_res\n",
		// both leaves
	})
	chain, err := Resolve("root", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"root", "vikunja", "pub_res"}
	if !reflect.DeepEqual(chain, want) {
		t.Errorf("chain = %v, want %v", chain, want)
	}
}

func TestResolve_DeclarationOrderFirstWins(t *testing.T) {
	// root -> (a, b); a -> (shared); b -> (shared).
	// `shared` is reached via `a` first, so its position is fixed after `a`
	// and the second encounter via `b` is a no-op (not a cycle).
	fetch := fetcherFromMap(map[string]string{
		"root":   "a\nb\n",
		"a":      "shared\n",
		"b":      "shared\n",
		"shared": "",
	})
	chain, err := Resolve("root", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"root", "a", "shared", "b"}
	if !reflect.DeepEqual(chain, want) {
		t.Errorf("chain = %v, want %v", chain, want)
	}
}

func TestResolve_CycleDirect(t *testing.T) {
	// a -> b -> a.
	fetch := fetcherFromMap(map[string]string{
		"a": "b\n",
		"b": "a\n",
	})
	_, err := Resolve("a", fetch)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error %v should mention cycle", err)
	}
	// Path should include all three hops.
	for _, want := range []string{"a", "b"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %v should contain %q", err, want)
		}
	}
}

func TestResolve_CycleSelfReference(t *testing.T) {
	// a -> a (self-include).
	fetch := fetcherFromMap(map[string]string{"a": "a\n"})
	_, err := Resolve("a", fetch)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error %v should mention cycle", err)
	}
}

func TestResolve_ExceedMaxDepth(t *testing.T) {
	// Linear chain root -> d1 -> d2 -> d3 -> d4. Depth to d4 is 4, which
	// exceeds MaxDepth=3.
	fetch := fetcherFromMap(map[string]string{
		"root": "d1\n",
		"d1":   "d2\n",
		"d2":   "d3\n",
		"d3":   "d4\n",
		"d4":   "",
	})
	_, err := Resolve("root", fetch)
	if err == nil {
		t.Fatal("expected depth-overflow error, got nil")
	}
	if !strings.Contains(err.Error(), "depth") {
		t.Errorf("error %v should mention depth", err)
	}
	// Path should name the offending link.
	if !strings.Contains(err.Error(), "d4") {
		t.Errorf("error %v should contain the overflowing folder d4", err)
	}
}

func TestResolve_MaxDepthBoundary(t *testing.T) {
	// Chain of exactly depth 3 must succeed.
	fetch := fetcherFromMap(map[string]string{
		"root": "d1\n",
		"d1":   "d2\n",
		"d2":   "d3\n",
		"d3":   "",
	})
	chain, err := Resolve("root", fetch)
	if err != nil {
		t.Fatalf("unexpected error at MaxDepth: %v", err)
	}
	want := []string{"root", "d1", "d2", "d3"}
	if !reflect.DeepEqual(chain, want) {
		t.Errorf("chain = %v, want %v", chain, want)
	}
}

func TestResolve_FetchError(t *testing.T) {
	sentinel := errors.New("boom")
	fetch := func(folder string) (string, bool, error) {
		if folder == "root" {
			return "child\n", true, nil
		}
		return "", false, sentinel
	}
	_, err := Resolve("root", fetch)
	if err == nil {
		t.Fatal("expected fetch error to propagate, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v should wrap sentinel", err)
	}
	if !strings.Contains(err.Error(), "child") {
		t.Errorf("error %v should name the folder that failed", err)
	}
}

func TestResolve_NilFetch(t *testing.T) {
	_, err := Resolve("root", nil)
	if err == nil {
		t.Fatal("expected error for nil fetch")
	}
}

func TestResolve_EmptyRoot(t *testing.T) {
	for _, root := range []string{"", "   ", "\t\n"} {
		_, err := Resolve(root, fetcherFromMap(nil))
		if err == nil {
			t.Errorf("root %q: expected error, got nil", root)
		}
	}
}

func TestResolve_DiamondDoesNotReportCycle(t *testing.T) {
	// Diamond: root -> (a, b); a -> shared; b -> shared. shared is reached
	// twice but not on the active stack the second time, so this must not
	// be reported as a cycle.
	fetch := fetcherFromMap(map[string]string{
		"root":   "a\nb\n",
		"a":      "shared\n",
		"b":      "shared\n",
		"shared": "",
	})
	_, err := Resolve("root", fetch)
	if err != nil {
		t.Fatalf("diamond should not be a cycle: %v", err)
	}
}

// sanity: make sure we are not accidentally exporting a surprising MaxDepth
// default that would silently widen scope later.
func TestMaxDepthIsThree(t *testing.T) {
	if MaxDepth != 3 {
		t.Fatalf("MaxDepth = %d, want 3 (MVP contract)", MaxDepth)
	}
}

// guard against someone ever adding a fmt verb that breaks path rendering.
func TestFormatPath(t *testing.T) {
	got := formatPath([]string{"root", "a", "b"})
	want := "root -> a -> b"
	if got != want {
		t.Errorf("formatPath = %q, want %q", got, want)
	}
	// sanity: %v renders as fmt.Sprintf("%v", ...) would, no panics
	_ = fmt.Sprintf("%v", got)
}
