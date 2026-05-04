package env

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

func TestParseEnvVars(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []EnvVar
		wantErr bool
	}{
		{name: "empty string", content: "", want: nil},
		{name: "single KEY=VALUE", content: "FOO=bar", want: []EnvVar{{Key: "FOO", Value: "bar"}}},
		{name: "multiple KEY=VALUE", content: "FOO=bar\nBAZ=qux", want: []EnvVar{{Key: "FOO", Value: "bar"}, {Key: "BAZ", Value: "qux"}}},
		{name: "export KEY=VALUE format", content: "export FOO=bar", want: []EnvVar{{Key: "FOO", Value: "bar"}}},
		{name: "double quoted value", content: `KEY="value"`, want: []EnvVar{{Key: "KEY", Value: "value"}}},
		{name: "single quoted value", content: "KEY='value'", want: []EnvVar{{Key: "KEY", Value: "value"}}},
		{name: "comment line", content: "# this is a comment", want: nil},
		{name: "empty lines skipped", content: "\n\n\n", want: nil},
		{name: "mixed comments and normal lines", content: "# comment\nFOO=bar\n# another comment\nBAZ=qux", want: []EnvVar{{Key: "FOO", Value: "bar"}, {Key: "BAZ", Value: "qux"}}},
		{name: "error no equals sign", content: "INVALID_LINE", wantErr: true},
		{name: "error equals at start", content: "=VALUE", wantErr: true},
		{name: "value contains equals sign", content: "KEY=a=b", want: []EnvVar{{Key: "KEY", Value: "a=b"}}},
		{name: "unmatched quotes preserved", content: `KEY="value`, want: []EnvVar{{Key: "KEY", Value: `"value`}}},
		{name: "whitespace only value", content: "KEY=  ", want: []EnvVar{{Key: "KEY", Value: ""}}},
		{name: "export with extra spaces", content: "export   FOO=bar", want: []EnvVar{{Key: "FOO", Value: "bar"}}},
		{name: "invalid env key", content: "BAD-KEY=value", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnvVars(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEnvVars() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("ParseEnvVars() got %d vars, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i].Key != tt.want[i].Key || got[i].Value != tt.want[i].Value {
					t.Errorf("ParseEnvVars()[%d] = {%q, %q}, want {%q, %q}", i, got[i].Key, got[i].Value, tt.want[i].Key, tt.want[i].Value)
				}
			}
		})
	}
}

func TestStripQuotes(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "double quotes", value: `"value"`, want: "value"},
		{name: "single quotes", value: "'value'", want: "value"},
		{name: "no quotes", value: "value", want: "value"},
		{name: "empty string", value: "", want: ""},
		{name: "single character", value: "a", want: "a"},
		{name: "mismatched quotes", value: `"value'`, want: `"value'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripQuotes(tt.value, 1)
			if got != tt.want {
				t.Errorf("stripQuotes(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestArtifactWriters(t *testing.T) {
	t.Run("sanitize folder name", func(t *testing.T) {
		got := sanitizeFolderName("prod/api keys")
		if got != "prod_api_keys" {
			t.Fatalf("sanitizeFolderName() = %q", got)
		}
	})

	t.Run("escape shell value", func(t *testing.T) {
		got := escapeShellValue("don't")
		if got != "don'\\''t" {
			t.Fatalf("escapeShellValue() = %q", got)
		}
	})

	t.Run("escape powershell value", func(t *testing.T) {
		got := escapePowerShellValue("$var's`test")
		if got != "`$var''s``test" {
			t.Fatalf("escapePowerShellValue() = %q", got)
		}
	})

	t.Run("valid env var name", func(t *testing.T) {
		if !validEnvVarName("MY_VAR") {
			t.Fatal("expected valid env var")
		}
		if validEnvVarName("MY-VAR") {
			t.Fatal("expected invalid env var")
		}
	})
}

func TestDeployAndRemoveArtifacts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st := &state.State{}
	deployer := NewDeployer(st)

	item := types.Item{ID: "env-1", Name: types.ReservedEnvNoteName, Notes: "FOO=bar\nBAR=baz\n"}
	entry, err := deployer.Deploy("prod", item)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}

	if len(st.Envs) != 1 {
		t.Fatalf("state env count = %d, want 1", len(st.Envs))
	}
	if entry.JSONPath == "" || entry.ShellPath == "" || entry.PowerShellPath == "" {
		t.Fatal("artifact paths should be populated")
	}

	jsonData, err := os.ReadFile(entry.JSONPath)
	if err != nil {
		t.Fatalf("read json artifact: %v", err)
	}
	var parsed map[string]string
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("parse json artifact: %v", err)
	}
	if parsed["FOO"] != "bar" || parsed["BAR"] != "baz" {
		t.Fatalf("unexpected json artifact: %+v", parsed)
	}

	shellData, err := os.ReadFile(entry.ShellPath)
	if err != nil {
		t.Fatalf("read shell artifact: %v", err)
	}
	if len(shellData) == 0 {
		t.Fatal("shell artifact should not be empty")
	}

	psData, err := os.ReadFile(entry.PowerShellPath)
	if err != nil {
		t.Fatalf("read powershell artifact: %v", err)
	}
	if len(psData) == 0 {
		t.Fatal("powershell artifact should not be empty")
	}

	if err := deployer.Remove(entry); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	for _, path := range []string{entry.JSONPath, entry.ShellPath, entry.PowerShellPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("artifact still exists: %s", path)
		}
	}
}

func TestDeployReplacesFolderEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st := &state.State{}
	deployer := NewDeployer(st)

	_, err := deployer.Deploy("prod", types.Item{ID: "env-1", Name: types.ReservedEnvNoteName, Notes: "FOO=bar"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = deployer.Deploy("prod", types.Item{ID: "env-2", Name: types.ReservedEnvNoteName, Notes: "BAR=baz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Envs) != 1 {
		t.Fatalf("state env count = %d, want 1", len(st.Envs))
	}
	if st.Envs[0].ItemID != "env-2" {
		t.Fatalf("item id = %q", st.Envs[0].ItemID)
	}
}

func TestArtifactPathsUseHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	jsonPath, shellPath, psPath, err := artifactPaths("prod")
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(home, envArtifactDir)
	if filepath.Dir(jsonPath) != base || filepath.Dir(shellPath) != base || filepath.Dir(psPath) != base {
		t.Fatalf("artifact paths not under %s", base)
	}
}

func TestFirstContributingSource(t *testing.T) {
	t.Run("returns first chain folder that actually contributed", func(t *testing.T) {
		result := MergeResult{
			Chain: []string{"proj-a", "shared-infra", "deep-lib"},
			Vars: []SourcedEnvVar{
				{EnvVar: EnvVar{Key: "K1", Value: "v"}, Source: "deep-lib"},
				{EnvVar: EnvVar{Key: "K2", Value: "v"}, Source: "shared-infra"},
			},
		}
		if got := firstContributingSource(result); got != "shared-infra" {
			t.Errorf("firstContributingSource = %q, want %q (chain order, not var order)", got, "shared-infra")
		}
	})

	t.Run("current folder included in chain but not in sources is skipped", func(t *testing.T) {
		result := MergeResult{
			Chain: []string{"proj-a", "shared"},
			Vars: []SourcedEnvVar{
				{EnvVar: EnvVar{Key: "K1"}, Source: "shared"},
			},
		}
		if got := firstContributingSource(result); got != "shared" {
			t.Errorf("firstContributingSource = %q, want %q", got, "shared")
		}
	})

	t.Run("empty vars returns empty string", func(t *testing.T) {
		result := MergeResult{Chain: []string{"proj-a"}, Vars: nil}
		if got := firstContributingSource(result); got != "" {
			t.Errorf("firstContributingSource = %q, want empty", got)
		}
	})
}

func TestDeployMergedAttribution(t *testing.T) {
	t.Run("hasCurrent=true leaves SourceFolder empty", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		st := &state.State{}
		d := NewDeployer(st)
		result := MergeResult{
			Chain: []string{"proj-a", "shared"},
			Vars: []SourcedEnvVar{
				{EnvVar: EnvVar{Key: "K1", Value: "v1"}, Source: "proj-a"},
				{EnvVar: EnvVar{Key: "K2", Value: "v2"}, Source: "shared"},
			},
		}
		entry, err := d.DeployMerged("proj-a", result, types.Item{ID: "env-item", Name: "pkv.env"}, true)
		if err != nil {
			t.Fatalf("DeployMerged: %v", err)
		}
		if entry.ItemID != "env-item" {
			t.Errorf("ItemID = %q, want %q", entry.ItemID, "env-item")
		}
		if entry.SourceFolder != "" {
			t.Errorf("SourceFolder = %q, want empty when chain[0] owns pkv.env", entry.SourceFolder)
		}
	})

	t.Run("hasCurrent=false records first contributing chain folder", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		st := &state.State{}
		d := NewDeployer(st)
		result := MergeResult{
			Chain: []string{"thin-proj", "shared-infra", "deep-lib"},
			Vars: []SourcedEnvVar{
				{EnvVar: EnvVar{Key: "K1", Value: "v1"}, Source: "deep-lib"},
				{EnvVar: EnvVar{Key: "K2", Value: "v2"}, Source: "shared-infra"},
			},
		}
		entry, err := d.DeployMerged("thin-proj", result, types.Item{}, false)
		if err != nil {
			t.Fatalf("DeployMerged: %v", err)
		}
		if entry.ItemID != "" {
			t.Errorf("ItemID = %q, want empty when chain[0] has no pkv.env", entry.ItemID)
		}
		if entry.Name != types.ReservedEnvNoteName {
			t.Errorf("Name = %q, want %q placeholder", entry.Name, types.ReservedEnvNoteName)
		}
		if entry.SourceFolder != "shared-infra" {
			t.Errorf("SourceFolder = %q, want %q", entry.SourceFolder, "shared-infra")
		}
	})
}
