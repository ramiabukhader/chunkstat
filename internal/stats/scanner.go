// Package stats scans directory trees and summarizes their files and lines.
package stats

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
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
	Root           string          `json:"root"`
	Files          int             `json:"files"`
	TotalLines     int             `json:"total_lines"`
	ByExtension    []ExtensionStat `json:"by_extension"`
	LargestFiles   []FileStat      `json:"largest_files"`
	IgnoredFolders []string        `json:"ignored_folders"`
}

// Scan walks root without following directory symlinks. It counts regular files,
// skips common dependency and build folders, and returns up to largestLimit files
// ordered by size. Binary files count as files but contribute zero lines.
func Scan(root string, largestLimit int) (Report, error) {
	if largestLimit < 0 {
		return Report{}, fmt.Errorf("largest-file limit must be zero or greater")
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
		Root:           absoluteRoot,
		ByExtension:    make([]ExtensionStat, 0),
		LargestFiles:   make([]FileStat, 0),
		IgnoredFolders: make([]string, 0),
	}
	byExtension := make(map[string]*ExtensionStat)
	files := make([]FileStat, 0)

	err = filepath.WalkDir(absoluteRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != absoluteRoot && isIgnoredDirectory(entry.Name()) {
				report.IgnoredFolders = append(report.IgnoredFolders, relativePath(absoluteRoot, path))
				return filepath.SkipDir
			}
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		lines, err := countLines(path)
		if err != nil {
			return fmt.Errorf("count lines in %s: %w", path, err)
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
			Path:  relativePath(absoluteRoot, path),
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
