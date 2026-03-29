package ssh

import (
	"fmt"
	"testing"
)

func TestRemoveManagedBlock(t *testing.T) {
	startFmt := "# >>> PKV MANAGED START: %s <<<"
	endFmt := "# >>> PKV MANAGED END: %s <<<"

	makeBlock := func(keyName, body string) string {
		return fmt.Sprintf(startFmt, keyName) + "\n" + body + "\n" + fmt.Sprintf(endFmt, keyName)
	}

	tests := []struct {
		name    string
		content string
		keyName string
		want    string
	}{
		{
			name:    "block in middle",
			content: "before\n" + makeBlock("mykey", "Host example.com\n    IdentityFile ~/.ssh/pkv_mykey") + "\nafter",
			keyName: "mykey",
			want:    "before\nafter",
		},
		{
			name:    "block at start",
			content: makeBlock("mykey", "Host example.com") + "\nafter",
			keyName: "mykey",
			want:    "after",
		},
		{
			name:    "block at end",
			content: "before\n" + makeBlock("mykey", "Host example.com"),
			keyName: "mykey",
			want:    "before",
		},
		{
			name:    "empty content",
			content: "",
			keyName: "mykey",
			want:    "",
		},
		{
			name:    "block not found",
			content: "some content\nother line",
			keyName: "mykey",
			want:    "some content\nother line",
		},
		{
			name:    "multiple blocks remove only matching",
			content: makeBlock("key1", "Host a.com") + "\nmiddle\n" + makeBlock("key2", "Host b.com"),
			keyName: "key1",
			want:    "middle\n" + makeBlock("key2", "Host b.com"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeManagedBlock(tt.content, tt.keyName)
			if got != tt.want {
				t.Errorf("removeManagedBlock() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestParseHostPort(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantPort string
	}{
		{name: "hostname only", input: "example.com", wantHost: "example.com", wantPort: ""},
		{name: "hostname with port", input: "example.com:2222", wantHost: "example.com", wantPort: "2222"},
		{name: "ipv6 bracketed no port", input: "[::1]", wantHost: "[::1]", wantPort: ""},
		{name: "ipv6 bracketed with port", input: "[::1]:2222", wantHost: "[::1]", wantPort: "2222"},
		{name: "ipv6 full bracketed with port", input: "[2001:db8::1]:22", wantHost: "[2001:db8::1]", wantPort: "22"},
		{name: "bare ipv6 multiple colons", input: "2001:db8::1", wantHost: "2001:db8::1", wantPort: ""},
		{name: "empty string", input: "", wantHost: "", wantPort: ""},
		{name: "simple host port", input: "host:22", wantHost: "host", wantPort: "22"},
		{name: "empty host with port", input: ":22", wantHost: "", wantPort: "22"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort := parseHostPort(tt.input)
			if gotHost != tt.wantHost || gotPort != tt.wantPort {
				t.Errorf("parseHostPort(%q) = (%q, %q), want (%q, %q)",
					tt.input, gotHost, gotPort, tt.wantHost, tt.wantPort)
			}
		})
	}
}

func TestCollapseBlankLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "multiple blank lines", input: "a\n\n\nb", want: "a\n\nb"},
		{name: "no consecutive blanks", input: "a\nb", want: "a\nb"},
		{name: "single blank line", input: "a\n\nb", want: "a\n\nb"},
		{name: "empty string", input: "", want: ""},
		{name: "only blank lines", input: "\n\n\n", want: ""},
		{name: "many blank lines", input: "a\n\n\n\n\nb", want: "a\n\nb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collapseBlankLines(tt.input)
			if got != tt.want {
				t.Errorf("collapseBlankLines(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
