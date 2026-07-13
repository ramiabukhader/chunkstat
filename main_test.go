package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramiabukhader/chunkstat/internal/stats"
)

func TestRunProducesJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "example.go"), []byte("package example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-json", "-top", "1", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}

	var report stats.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("JSON output is invalid: %v\n%s", err, stdout.String())
	}
	if report.Files != 1 || report.TotalLines != 1 {
		t.Errorf("report = %#v, want one file and one line", report)
	}
}

func TestRunSupportsRepeatedExclusions(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name := range map[string]struct{}{"keep.go": {}, "skip.tmp": {}, "generated/code.go": {}} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("one\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	args := []string{"-json", "--exclude", "*.tmp", "--exclude", `generated\`, root}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	var report stats.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Files != 1 || len(report.ExcludedPaths) != 2 {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunValidatesArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "negative top", args: []string{"-top", "-1"}, want: "must be zero or greater"},
		{name: "extra directory", args: []string{"one", "two"}, want: "Usage: chunkstat"},
		{name: "unknown flag", args: []string{"-unknown"}, want: "flag provided but not defined"},
		{name: "unsafe exclude", args: []string{"--exclude", "../outside"}, want: "must not traverse"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != 2 {
				t.Fatalf("run() code = %d, want 2", code)
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), test.want)
			}
		})
	}
}

func TestRunHelpSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() code = %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "Usage: chunkstat") {
		t.Errorf("help output = %q", stderr.String())
	}
}

func TestFormatBytes(t *testing.T) {
	tests := map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KiB", 1024 * 1024: "1.0 MiB"}
	for input, want := range tests {
		if got := formatBytes(input); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", input, got, want)
		}
	}
}
