package ssh

import "testing"

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "lowercase with hyphen", input: "my-key", want: "my-key"},
		{name: "underscore and digits", input: "my_key123", want: "my_key123"},
		{name: "space to underscore", input: "My Key", want: "My_Key"},
		{name: "space and special chars", input: "My Key!@#", want: "My_Key"},
		{name: "empty string", input: "", want: ""},
		{name: "only special chars", input: "!@#$%", want: ""},
		{name: "multiple words", input: "hello world", want: "hello_world"},
		{name: "word space digit", input: "key 1", want: "key_1"},
		{name: "uppercase only", input: "ABC", want: "ABC"},
		{name: "mixed hyphen underscore", input: "a-b_c", want: "a-b_c"},
		{name: "dot removed", input: "test.key", want: "testkey"},
		{name: "unicode removed", input: "中文key", want: "key"},
		{name: "leading trailing spaces", input: "  spaces  ", want: "__spaces__"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
