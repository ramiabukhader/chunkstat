package stats

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestScanSummarizesFilesAndSkipsIgnoredFolders(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	writeTestFile(t, root, "helper.GO", "package main")
	writeTestFile(t, root, "notes.txt", "first\nsecond")
	writeTestFile(t, root, "LICENSE", "one line\n")
	writeTestFile(t, root, "node_modules/ignored.js", "one\ntwo\nthree\n")
	writeTestFile(t, root, ".git/config", "ignored\n")

	report, err := Scan(root, 3)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if report.Files != 4 {
		t.Errorf("Files = %d, want 4", report.Files)
	}
	if report.TotalLines != 7 {
		t.Errorf("TotalLines = %d, want 7", report.TotalLines)
	}
	wantExtensions := []ExtensionStat{
		{Extension: ".go", Files: 2, Lines: 4},
		{Extension: ".txt", Files: 1, Lines: 2},
		{Extension: "[no extension]", Files: 1, Lines: 1},
	}
	if !reflect.DeepEqual(report.ByExtension, wantExtensions) {
		t.Errorf("ByExtension = %#v, want %#v", report.ByExtension, wantExtensions)
	}
	wantIgnored := []string{".git", "node_modules"}
	if !reflect.DeepEqual(report.IgnoredFolders, wantIgnored) {
		t.Errorf("IgnoredFolders = %#v, want %#v", report.IgnoredFolders, wantIgnored)
	}
	if len(report.LargestFiles) != 3 {
		t.Fatalf("len(LargestFiles) = %d, want 3", len(report.LargestFiles))
	}
	for index := 1; index < len(report.LargestFiles); index++ {
		if report.LargestFiles[index-1].Bytes < report.LargestFiles[index].Bytes {
			t.Errorf("LargestFiles is not sorted descending: %#v", report.LargestFiles)
		}
	}
}

func TestScanCountsBinaryFileAsZeroLines(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "data.bin", string([]byte{1, 2, 0, '\n', 3}))

	report, err := Scan(root, 10)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if report.Files != 1 || report.TotalLines != 0 {
		t.Errorf("report = %#v, want one file and zero lines", report)
	}
	if got := report.ByExtension[0]; got != (ExtensionStat{Extension: ".bin", Files: 1, Lines: 0}) {
		t.Errorf("ByExtension[0] = %#v", got)
	}
}

func TestScanRejectsInvalidInputs(t *testing.T) {
	if _, err := Scan(t.TempDir(), -1); err == nil {
		t.Error("Scan() with negative limit returned nil error")
	}

	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(file, 1); err == nil {
		t.Error("Scan() with file root returned nil error")
	}
}

func TestScanAppliesPortableExclusionPatterns(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "keep.go", "one\n")
	writeTestFile(t, root, "generated/code.go", "one\ntwo\n")
	writeTestFile(t, root, "nested/cache.tmp", "three\n")

	report, err := ScanWithOptions(root, ScanOptions{
		LargestLimit:    10,
		ExcludePatterns: []string{`generated`, `nested\*.tmp`},
	})
	if err != nil {
		t.Fatalf("ScanWithOptions() error = %v", err)
	}
	if report.Files != 1 || report.TotalLines != 1 {
		t.Fatalf("report = %#v, want only keep.go", report)
	}
	wantExcluded := []string{"generated", "nested/cache.tmp"}
	if !reflect.DeepEqual(report.ExcludedPaths, wantExcluded) {
		t.Fatalf("ExcludedPaths = %#v, want %#v", report.ExcludedPaths, wantExcluded)
	}
}

func TestNormalizeExcludePatternRejectsUnsafeValues(t *testing.T) {
	for _, pattern := range []string{"", "../outside", `..\outside`, `C:\outside`, filepath.Join(t.TempDir(), "absolute")} {
		if _, err := NormalizeExcludePattern(pattern); err == nil {
			t.Errorf("NormalizeExcludePattern(%q) error = nil", pattern)
		}
	}
	if got, err := NormalizeExcludePattern(`generated\*.go`); err != nil || got != "generated/*.go" {
		t.Fatalf("NormalizeExcludePattern() = %q, %v", got, err)
	}
}

func TestScanPreservesPartialResultsAndSanitizesReadErrors(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "z-good.txt", "one\ntwo\n")
	writeTestFile(t, root, "a-bad.txt", "secret fixture must not appear")

	report, err := scanWithOptions(root, ScanOptions{LargestLimit: 10}, func(filePath string) (int, error) {
		if filepath.Base(filePath) == "a-bad.txt" {
			return 0, errors.New("synthetic failure containing " + filePath)
		}
		return countLines(filePath)
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Version != "1" || report.Files != 1 || report.TotalLines != 2 {
		t.Fatalf("report = %#v", report)
	}
	wantErrors := []ScanIssue{{Kind: "read-error", Path: "a-bad.txt", Message: "cannot read file"}}
	if !reflect.DeepEqual(report.Errors, wantErrors) {
		t.Fatalf("Errors = %#v, want %#v", report.Errors, wantErrors)
	}
	encoded := fmt.Sprintf("%#v", report.Errors)
	if strings.Contains(encoded, root) || strings.Contains(encoded, "secret fixture") {
		t.Fatalf("error report leaked root/content: %s", encoded)
	}
}

func TestScanInitializesVersionedEmptyArrays(t *testing.T) {
	report, err := Scan(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Version != "1" || report.ByExtension == nil || report.LargestFiles == nil || report.IgnoredFolders == nil || report.ExcludedPaths == nil || report.Errors == nil {
		t.Fatalf("report contains unstable nil fields: %#v", report)
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{name: "empty", content: "", want: 0},
		{name: "one without newline", content: "hello", want: 1},
		{name: "one with newline", content: "hello\n", want: 1},
		{name: "multiple", content: "one\ntwo\nthree", want: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.txt")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := countLines(path)
			if err != nil {
				t.Fatalf("countLines() error = %v", err)
			}
			if got != test.want {
				t.Errorf("countLines() = %d, want %d", got, test.want)
			}
		})
	}
}

func writeTestFile(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
