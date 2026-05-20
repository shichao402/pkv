package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	bwtypes "github.com/shichao402/pkv/internal/bw/types"
)

type captureReporter struct {
	infos  []string
	warns  []string
	errors []string
}

func (r *captureReporter) Info(message string) {
	r.infos = append(r.infos, message)
}

func (r *captureReporter) Infof(format string, args ...any) {
	r.infos = append(r.infos, strings.TrimSuffix(sprintf(format, args...), "\n"))
}

func (r *captureReporter) Warn(message string) {
	r.warns = append(r.warns, message)
}

func (r *captureReporter) Warnf(format string, args ...any) {
	r.warns = append(r.warns, strings.TrimSuffix(sprintf(format, args...), "\n"))
}

func (r *captureReporter) Error(message string) {
	r.errors = append(r.errors, message)
}

func (r *captureReporter) Errorf(format string, args ...any) {
	r.errors = append(r.errors, strings.TrimSuffix(sprintf(format, args...), "\n"))
}

func sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

type fakeAddFolderClient struct {
	folders       []bwtypes.Folder
	createdFolder bwtypes.Folder
	calls         []string
}

func (c *fakeAddFolderClient) EnsureUnlocked() (string, error) {
	c.calls = append(c.calls, "unlock")
	return "session", nil
}

func (c *fakeAddFolderClient) Sync(string) error {
	c.calls = append(c.calls, "sync")
	return nil
}

func (c *fakeAddFolderClient) ListFolders(string) ([]bwtypes.Folder, error) {
	c.calls = append(c.calls, "list")
	return c.folders, nil
}

func (c *fakeAddFolderClient) CreateFolder(_ string, name string) (bwtypes.Folder, error) {
	c.calls = append(c.calls, "create:"+name)
	if c.createdFolder.Name == "" {
		c.createdFolder.Name = name
	}
	return c.createdFolder, nil
}

func TestAddFolderRejectsEmptyNameBeforeClientCalls(t *testing.T) {
	client := &fakeAddFolderClient{}
	_, err := addFolderWithClient(context.Background(), client, AddFolderParams{Name: "  "}, nil)
	if err == nil || !strings.Contains(err.Error(), "folder name is required") {
		t.Fatalf("AddFolder empty name error = %v, want required", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("client calls = %v, want none", client.calls)
	}
}

func TestAddFolderRejectsDuplicateName(t *testing.T) {
	client := &fakeAddFolderClient{folders: []bwtypes.Folder{{ID: "folder-1", Name: "Prod"}}}
	_, err := addFolderWithClient(context.Background(), client, AddFolderParams{Name: "prod"}, nil)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("AddFolder duplicate error = %v, want already exists", err)
	}
	if strings.Join(client.calls, ",") != "unlock,sync,list" {
		t.Fatalf("client calls = %v, want unlock/sync/list only", client.calls)
	}
}

func TestAddFolderCreatesFolder(t *testing.T) {
	client := &fakeAddFolderClient{createdFolder: bwtypes.Folder{ID: "folder-created", Name: "prod"}}
	reporter := &captureReporter{}
	result, err := addFolderWithClient(context.Background(), client, AddFolderParams{Name: " prod "}, reporter)
	if err != nil {
		t.Fatalf("AddFolder returned error: %v", err)
	}
	if result.Folder.ID != "folder-created" || result.Folder.Name != "prod" {
		t.Fatalf("AddFolder result = %+v, want created prod folder", result.Folder)
	}
	if strings.Join(client.calls, ",") != "unlock,sync,list,create:prod" {
		t.Fatalf("client calls = %v", client.calls)
	}
	if got := strings.Join(reporter.infos, "\n"); !strings.Contains(got, "Folder 'prod' created") {
		t.Fatalf("reporter output = %q, want created message", got)
	}
}

func TestCleanEnvNoEntries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reporter := &captureReporter{}

	result, err := CleanEnv(context.Background(), CleanParams{Folder: "prod"}, reporter)
	if err != nil {
		t.Fatalf("CleanEnv returned error: %v", err)
	}
	if result.Cleaned != 0 {
		t.Fatalf("CleanEnv cleaned %d entries, want 0", result.Cleaned)
	}
	joined := strings.Join(reporter.infos, "\n")
	if !strings.Contains(joined, "No env artifacts found for folder 'prod'.") {
		t.Fatalf("CleanEnv output = %q, want missing-artifacts message", joined)
	}
}

func TestCleanRejectsUnknownKind(t *testing.T) {
	_, err := Clean(context.Background(), CleanParams{Folder: "prod", Kind: "bad"}, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown resource type") {
		t.Fatalf("Clean unknown kind error = %v, want unknown resource type", err)
	}
}

func TestCleanEnvHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := CleanEnv(ctx, CleanParams{Folder: "prod"}, nil)
	if err == nil {
		t.Fatal("CleanEnv canceled context error = nil, want error")
	}
}
