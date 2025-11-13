// Package stats scans directory trees and summarizes their files and lines.
package stats

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var ignoredDirectoryNames = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"bin":          {},
	"obj":          {},
	"vendor":       {},
}

// ExtensionStat contains counts for one file extension.
type ExtensionStat struct {
	Extension string `json:"extension"`
	Files     int    `json:"files"`
	Lines     int    `json:"lines"`
}

// FileStat describes a file included in the largest-files list.
type FileStat struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Lines int    `json:"lines"`
}

// Report is the result of scanning a directory tree.
type Report struct {
	Version        string          `json:"version"`
	Root           string          `json:"root"`
	Files          int             `json:"files"`
	TotalLines     int             `json:"total_lines"`
	ByExtension    []ExtensionStat `json:"by_extension"`
	LargestFiles   []FileStat      `json:"largest_files"`
	IgnoredFolders []string        `json:"ignored_folders"`
	ExcludedPaths  []string        `json:"excluded_paths"`
	Errors         []ScanIssue     `json:"errors"`
}

// ScanIssue describes one path that could not be scanned completely.
type ScanIssue struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ScanOptions controls result size and repository-relative exclusions.
type ScanOptions struct {
	LargestLimit    int
	ExcludePatterns []string
}

// Scan walks root without following directory symlinks. It counts regular files,
// skips common dependency and build folders, and returns up to largestLimit files
// ordered by size. Binary files count as files but contribute zero lines.
func Scan(root string, largestLimit int) (Report, error) {
	return ScanWithOptions(root, ScanOptions{LargestLimit: largestLimit})
}

// ScanWithOptions scans root with validated, slash-normalized exclusion globs.
func ScanWithOptions(root string, options ScanOptions) (Report, error) {
	return scanWithOptions(root, options, countLines)
}

func scanWithOptions(root string, options ScanOptions, lineCounter func(string) (int, error)) (Report, error) {
	largestLimit := options.LargestLimit
	if largestLimit < 0 {
		return Report{}, fmt.Errorf("largest-file limit must be zero or greater")
	}
	patterns := make([]string, 0, len(options.ExcludePatterns))
	for _, pattern := range options.ExcludePatterns {
		normalized, err := NormalizeExcludePattern(pattern)
		if err != nil {
			return Report{}, err
		}
		patterns = append(patterns, normalized)
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return Report{}, fmt.Errorf("resolve root: %w", err)
	}
	rootInfo, err := os.Stat(absoluteRoot)
	if err != nil {
		return Report{}, fmt.Errorf("open root: %w", err)
	}
	if !rootInfo.IsDir() {
		return Report{}, fmt.Errorf("root is not a directory: %s", absoluteRoot)
	}

	report := Report{
		Version:        "1",
		Root:           absoluteRoot,
		ByExtension:    make([]ExtensionStat, 0),
		LargestFiles:   make([]FileStat, 0),
		IgnoredFolders: make([]string, 0),
		ExcludedPaths:  make([]string, 0),
		Errors:         make([]ScanIssue, 0),
	}
	byExtension := make(map[string]*ExtensionStat)
	files := make([]FileStat, 0)

	err = filepath.WalkDir(absoluteRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == absoluteRoot {
				return walkErr
			}
			report.Errors = append(report.Errors, ScanIssue{
				Kind: "walk-error", Path: portableRelativePath(absoluteRoot, path), Message: "cannot access path",
			})
			return nil
		}
		if entry.IsDir() {
			if path != absoluteRoot && isIgnoredDirectory(entry.Name()) {
				report.IgnoredFolders = append(report.IgnoredFolders, relativePath(absoluteRoot, path))
				return filepath.SkipDir
			}
			if path != absoluteRoot {
				relative := filepath.ToSlash(relativePath(absoluteRoot, path))
				if matchesExclusion(relative, patterns) {
					report.ExcludedPaths = append(report.ExcludedPaths, relative)
					return filepath.SkipDir
				}
			}
			return nil
		}
		relative := filepath.ToSlash(relativePath(absoluteRoot, path))
		if matchesExclusion(relative, patterns) {
			report.ExcludedPaths = append(report.ExcludedPaths, relative)
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			report.Errors = append(report.Errors, ScanIssue{
				Kind: "metadata-error", Path: portableRelativePath(absoluteRoot, path), Message: "cannot inspect file metadata",
			})
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		lines, err := lineCounter(path)
		if err != nil {
			report.Errors = append(report.Errors, ScanIssue{
				Kind: "read-error", Path: relative, Message: "cannot read file",
			})
			return nil
		}
		extension := extensionFor(entry.Name())
		group, ok := byExtension[extension]
		if !ok {
			group = &ExtensionStat{Extension: extension}
			byExtension[extension] = group
		}
		group.Files++
		group.Lines += lines
		report.Files++
		report.TotalLines += lines
		files = append(files, FileStat{
			Path:  relative,
			Bytes: info.Size(),
			Lines: lines,
		})
		return nil
	})
	if err != nil {
		return Report{}, fmt.Errorf("scan directory: %w", err)
	}

	for _, group := range byExtension {
		report.ByExtension = append(report.ByExtension, *group)
	}
	sort.Slice(report.ByExtension, func(i, j int) bool {
		return report.ByExtension[i].Extension < report.ByExtension[j].Extension
	})
	sort.Strings(report.IgnoredFolders)
	sort.Strings(report.ExcludedPaths)
	sort.Slice(report.Errors, func(i, j int) bool {
		if report.Errors[i].Path == report.Errors[j].Path {
			return report.Errors[i].Kind < report.Errors[j].Kind
		}
		return report.Errors[i].Path < report.Errors[j].Path
	})
	sort.Slice(files, func(i, j int) bool {
		if files[i].Bytes == files[j].Bytes {
			return files[i].Path < files[j].Path
		}
		return files[i].Bytes > files[j].Bytes
	})
	if largestLimit > len(files) {
		largestLimit = len(files)
	}
	report.LargestFiles = append(report.LargestFiles, files[:largestLimit]...)

	return report, nil
}

func portableRelativePath(root, filePath string) string {
	return filepath.ToSlash(relativePath(root, filePath))
}

// NormalizeExcludePattern validates a pattern and returns portable slash form.
func NormalizeExcludePattern(patternValue string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(patternValue), "\\", "/")
	if normalized == "" {
		return "", fmt.Errorf("exclude pattern must not be empty")
	}
	if filepath.IsAbs(patternValue) || path.IsAbs(normalized) || filepath.VolumeName(patternValue) != "" || hasWindowsDrive(normalized) {
		return "", fmt.Errorf("exclude pattern must be repository-relative: %q", patternValue)
	}
	normalized = strings.TrimPrefix(normalized, "./")
	cleaned := path.Clean(normalized)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("exclude pattern must not traverse outside the scan root: %q", patternValue)
	}
	if _, err := path.Match(cleaned, "validation"); err != nil {
		return "", fmt.Errorf("invalid exclude pattern %q: %w", patternValue, err)
	}
	return cleaned, nil
}

func hasWindowsDrive(value string) bool {
	return len(value) >= 2 && value[1] == ':' && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z'))
}

func matchesExclusion(relative string, patterns []string) bool {
	for _, patternValue := range patterns {
		matched, _ := path.Match(patternValue, relative)
		if matched {
			return true
		}
		if !strings.ContainsAny(patternValue, "*?[") && strings.HasPrefix(relative, patternValue+"/") {
			return true
		}
	}
	return false
}

func isIgnoredDirectory(name string) bool {
	_, ignored := ignoredDirectoryNames[strings.ToLower(name)]
	return ignored
}

func extensionFor(name string) string {
	extension := strings.ToLower(filepath.Ext(name))
	if extension == "" {
		return "[no extension]"
	}
	return extension
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return relative
}

func countLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	buffer := make([]byte, 32*1024)
	lines := 0
	bytesRead := 0
	var lastByte byte
	for {
		count, readErr := reader.Read(buffer)
		if count > 0 {
			chunk := buffer[:count]
			if bytes.IndexByte(chunk, 0) >= 0 {
				return 0, nil
			}
			lines += bytes.Count(chunk, []byte{'\n'})
			lastByte = chunk[count-1]
			bytesRead += count
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, readErr
		}
	}
	if bytesRead > 0 && lastByte != '\n' {
		lines++
	}
	return lines, nil
}
