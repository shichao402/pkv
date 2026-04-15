package types

import (
	"testing"
)

func TestGetCustomField(t *testing.T) {
	tests := []struct {
		name   string
		item   Item
		field  string
		expect string
	}{
		{
			name: "field exists",
			item: Item{
				Fields: []CustomField{{Name: "key", Value: "value"}},
			},
			field:  "key",
			expect: "value",
		},
		{
			name: "field not found",
			item: Item{
				Fields: []CustomField{{Name: "key", Value: "value"}},
			},
			field:  "missing",
			expect: "",
		},
		{
			name:   "empty fields list",
			item:   Item{},
			field:  "key",
			expect: "",
		},
		{
			name: "multiple fields find match",
			item: Item{
				Fields: []CustomField{
					{Name: "a", Value: "1"},
					{Name: "b", Value: "2"},
					{Name: "c", Value: "3"},
				},
			},
			field:  "b",
			expect: "2",
		},
		{
			name: "multiple fields same name returns first",
			item: Item{
				Fields: []CustomField{
					{Name: "dup", Value: "first"},
					{Name: "dup", Value: "second"},
				},
			},
			field:  "dup",
			expect: "first",
		},
		{
			name: "empty field name",
			item: Item{
				Fields: []CustomField{{Name: "key", Value: "value"}},
			},
			field:  "",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.item.GetCustomField(tt.field)
			if got != tt.expect {
				t.Errorf("GetCustomField(%q) = %q, want %q", tt.field, got, tt.expect)
			}
		})
	}
}

func TestPKVType(t *testing.T) {
	tests := []struct {
		name   string
		item   Item
		expect string
	}{
		{
			name: "pkv_type=env",
			item: Item{
				Fields: []CustomField{{Name: PKVFieldName, Value: "env"}},
			},
			expect: "env",
		},
		{
			name: "pkv_type=other",
			item: Item{
				Fields: []CustomField{{Name: PKVFieldName, Value: "other"}},
			},
			expect: "other",
		},
		{
			name:   "no pkv_type field",
			item:   Item{},
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.item.PKVType()
			if got != tt.expect {
				t.Errorf("PKVType() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestIsEnv(t *testing.T) {
	tests := []struct {
		name   string
		item   Item
		expect bool
	}{
		{
			name: "pkv_type=env returns true",
			item: Item{
				Fields: []CustomField{{Name: PKVFieldName, Value: PKVTypeEnv}},
			},
			expect: true,
		},
		{
			name: "pkv_type=other returns false",
			item: Item{
				Fields: []CustomField{{Name: PKVFieldName, Value: "other"}},
			},
			expect: false,
		},
		{
			name:   "no fields returns false",
			item:   Item{},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.item.IsEnv()
			if got != tt.expect {
				t.Errorf("IsEnv() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestNormalizeNoteStrategy(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "empty", input: "", expect: ""},
		{name: "keeps canonical value", input: "mise_conf_d", expect: "mise_conf_d"},
		{name: "normalizes hyphen and case", input: "  MISE-CONF-D ", expect: "mise_conf_d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeNoteStrategy(tt.input)
			if got != tt.expect {
				t.Fatalf("NormalizeNoteStrategy(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestNoteStrategy(t *testing.T) {
	tests := []struct {
		name   string
		item   Item
		expect string
	}{
		{name: "default strategy", item: Item{}, expect: NoteStrategyFile},
		{name: "custom strategy", item: Item{Fields: []CustomField{{Name: PKVNoteStrategyFieldName, Value: "MISE-CONF-D"}}}, expect: NoteStrategyMiseConfD},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.item.NoteStrategy()
			if got != tt.expect {
				t.Fatalf("NoteStrategy() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestNoteTargetPath(t *testing.T) {
	item := Item{Fields: []CustomField{{Name: PKVNoteTargetFieldName, Value: "  .config/mise/conf.d/test.toml  "}}}
	if got := item.NoteTargetPath(); got != ".config/mise/conf.d/test.toml" {
		t.Fatalf("NoteTargetPath() = %q", got)
	}
}

func TestGetHosts(t *testing.T) {
	tests := []struct {
		name   string
		item   Item
		expect []string
	}{
		{
			name:   "multiple lines",
			item:   Item{Notes: "host1\nhost2"},
			expect: []string{"host1", "host2"},
		},
		{
			name:   "empty notes",
			item:   Item{Notes: ""},
			expect: nil,
		},
		{
			name:   "lines with empty lines",
			item:   Item{Notes: "host1\n\nhost2"},
			expect: []string{"host1", "host2"},
		},
		{
			name:   "single line",
			item:   Item{Notes: "host1"},
			expect: []string{"host1"},
		},
		{
			name:   "lines with spaces",
			item:   Item{Notes: "  host1  \n  host2  "},
			expect: []string{"host1", "host2"},
		},
		{
			name:   "trailing newline",
			item:   Item{Notes: "host1\n"},
			expect: []string{"host1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.item.GetHosts()
			if !stringSliceEqual(got, tt.expect) {
				t.Errorf("GetHosts() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{
			name:   "multiple lines",
			input:  "a\nb\nc",
			expect: []string{"a", "b", "c"},
		},
		{
			name:   "single line no newline",
			input:  "abc",
			expect: []string{"abc"},
		},
		{
			name:   "empty string",
			input:  "",
			expect: nil,
		},
		{
			name:   "trailing newline",
			input:  "a\n",
			expect: []string{"a"},
		},
		{
			name:   "only newline",
			input:  "\n",
			expect: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if !stringSliceEqual(got, tt.expect) {
				t.Errorf("splitLines(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "spaces around",
			input:  "  hello  ",
			expect: "hello",
		},
		{
			name:   "no spaces",
			input:  "hello",
			expect: "hello",
		},
		{
			name:   "empty string",
			input:  "",
			expect: "",
		},
		{
			name:   "tabs around",
			input:  "\thello\t",
			expect: "hello",
		},
		{
			name:   "only spaces",
			input:  "  ",
			expect: "",
		},
		{
			name:   "carriage return and newline",
			input:  "\r\n",
			expect: "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimSpace(tt.input)
			if got != tt.expect {
				t.Errorf("trimSpace(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

// stringSliceEqual compares two string slices, treating nil and empty slice as equal.
func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		// Both nil or both empty — but distinguish nil vs empty when both lengths are 0
		if a == nil && b == nil {
			return true
		}
		if a == nil || b == nil {
			// One is nil, other is empty — for test purposes check length only
			return len(a) == len(b)
		}
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
