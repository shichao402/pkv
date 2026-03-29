package ssh

import "testing"

func TestRemoveKnownHostsBlock(t *testing.T) {
	const start = "# >>> PKV MANAGED START <<<"
	const end = "# >>> PKV MANAGED END <<<"

	makeBlock := func(body string) string {
		return start + "\n" + body + "\n" + end
	}

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "block in middle",
			content: "manual-host ssh-rsa AAA\n" + makeBlock("example.com ssh-rsa BBB") + "\nother-host ssh-rsa CCC",
			want:    "manual-host ssh-rsa AAA\nother-host ssh-rsa CCC",
		},
		{
			name:    "block at start",
			content: makeBlock("example.com ssh-rsa BBB") + "\nother-host ssh-rsa CCC",
			want:    "other-host ssh-rsa CCC",
		},
		{
			name:    "block at end",
			content: "manual-host ssh-rsa AAA\n" + makeBlock("example.com ssh-rsa BBB"),
			want:    "manual-host ssh-rsa AAA",
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name:    "block not found",
			content: "manual-host ssh-rsa AAA\nother-host ssh-rsa CCC",
			want:    "manual-host ssh-rsa AAA\nother-host ssh-rsa CCC",
		},
		{
			name:    "only pkv block",
			content: makeBlock("example.com ssh-rsa BBB"),
			want:    "",
		},
		{
			name:    "multi line block content",
			content: "before\n" + makeBlock("host1 ssh-rsa AAA\nhost2 ssh-ed25519 BBB\nhost3 ecdsa-sha2 CCC") + "\nafter",
			want:    "before\nafter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeKnownHostsBlock(tt.content)
			if got != tt.want {
				t.Errorf("removeKnownHostsBlock() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}
