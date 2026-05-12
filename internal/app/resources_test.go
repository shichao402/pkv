package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
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
