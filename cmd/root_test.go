package cmd

import "testing"

func TestDecideEntryMode(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		stdinTTY  bool
		stdoutTTY bool
		stderrTTY bool
		env       map[string]string
		want      entryMode
	}{
		{
			name:      "arguments force cli",
			args:      []string{"list"},
			stdinTTY:  true,
			stdoutTTY: true,
			stderrTTY: true,
			want:      entryModeCLI,
		},
		{
			name:      "no tui environment forces cli",
			stdinTTY:  true,
			stdoutTTY: true,
			stderrTTY: true,
			env:       map[string]string{"PKV_NO_TUI": "1"},
			want:      entryModeCLI,
		},
		{
			name:      "dumb terminal forces cli",
			stdinTTY:  true,
			stdoutTTY: true,
			stderrTTY: true,
			env:       map[string]string{"TERM": "dumb"},
			want:      entryModeCLI,
		},
		{
			name:      "non tty stdin forces cli",
			stdinTTY:  false,
			stdoutTTY: true,
			stderrTTY: true,
			want:      entryModeCLI,
		},
		{
			name:      "non tty stdout forces cli",
			stdinTTY:  true,
			stdoutTTY: false,
			stderrTTY: true,
			want:      entryModeCLI,
		},
		{
			name:      "non tty stderr forces cli",
			stdinTTY:  true,
			stdoutTTY: true,
			stderrTTY: false,
			want:      entryModeCLI,
		},
		{
			name:      "interactive terminal launches tui",
			stdinTTY:  true,
			stdoutTTY: true,
			stderrTTY: true,
			want:      entryModeTUI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideEntryMode(tt.args, tt.stdinTTY, tt.stdoutTTY, tt.stderrTTY, func(key string) string {
				return tt.env[key]
			})
			if got != tt.want {
				t.Fatalf("decideEntryMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTerminalNil(t *testing.T) {
	if isTerminal(nil) {
		t.Fatal("isTerminal(nil) = true, want false")
	}
}
