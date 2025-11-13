package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ramiabukhader/chunkstat/internal/stats"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithScanner(args, stdout, stderr, stats.ScanWithOptions)
}

func runWithScanner(args []string, stdout, stderr io.Writer, scan func(string, stats.ScanOptions) (stats.Report, error)) int {
	flags := flag.NewFlagSet("chunkstat", flag.ContinueOnError)
	flags.SetOutput(stderr)
	top := flags.Int("top", 10, "number of largest files to display")
	asJSON := flags.Bool("json", false, "print the report as JSON")
	var exclusions excludeValues
	flags.Var(&exclusions, "exclude", "repository-relative exclusion pattern (repeatable)")
	failOnErrors := flags.Bool("fail-on-errors", false, "exit 3 when a scan completes with path errors")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: chunkstat [flags] [directory]")
		fmt.Fprintln(stderr, "\nScan a directory and summarize file and line counts.")
		fmt.Fprintln(stderr, "\nFlags:")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *top < 0 {
		fmt.Fprintln(stderr, "chunkstat: -top must be zero or greater")
		return 2
	}
	if flags.NArg() > 1 {
		flags.Usage()
		return 2
	}

	root := "."
	if flags.NArg() == 1 {
		root = flags.Arg(0)
	}

	report, err := scan(root, stats.ScanOptions{
		LargestLimit:    *top,
		ExcludePatterns: exclusions,
	})
	if err != nil {
		fmt.Fprintf(stderr, "chunkstat: %v\n", err)
		return 1
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "chunkstat: write report: %v\n", err)
			return 1
		}
		if *failOnErrors && len(report.Errors) > 0 {
			return 3
		}
		return 0
	}

	printReport(stdout, report)
	if *failOnErrors && len(report.Errors) > 0 {
		return 3
	}
	return 0
}

func printReport(output io.Writer, report stats.Report) {
	fmt.Fprintf(output, "Directory:       %s\n", report.Root)
	fmt.Fprintf(output, "Files:           %d\n", report.Files)
	fmt.Fprintf(output, "Total lines:     %d\n", report.TotalLines)
	fmt.Fprintf(output, "Ignored folders: %d\n", len(report.IgnoredFolders))
	fmt.Fprintf(output, "Excluded paths:  %d\n", len(report.ExcludedPaths))
	fmt.Fprintf(output, "Scan errors:     %d\n", len(report.Errors))

	fmt.Fprintln(output, "\nBy extension:")
	fmt.Fprintf(output, "  %-16s %10s %12s\n", "EXTENSION", "FILES", "LINES")
	for _, extension := range report.ByExtension {
		fmt.Fprintf(output, "  %-16s %10d %12d\n", extension.Extension, extension.Files, extension.Lines)
	}

	if len(report.LargestFiles) > 0 {
		fmt.Fprintln(output, "\nLargest files:")
		fmt.Fprintf(output, "  %-12s %12s  %s\n", "SIZE", "LINES", "PATH")
		for _, file := range report.LargestFiles {
			fmt.Fprintf(output, "  %-12s %12d  %s\n", formatBytes(file.Bytes), file.Lines, filepath.ToSlash(file.Path))
		}
	}

	if len(report.IgnoredFolders) > 0 {
		fmt.Fprintln(output, "\nIgnored folder paths:")
		for _, path := range report.IgnoredFolders {
			fmt.Fprintf(output, "  %s\n", filepath.ToSlash(path))
		}
	}

	if len(report.ExcludedPaths) > 0 {
		fmt.Fprintln(output, "\nExcluded paths:")
		for _, excluded := range report.ExcludedPaths {
			fmt.Fprintf(output, "  %s\n", filepath.ToSlash(excluded))
		}
	}

	if len(report.Errors) > 0 {
		fmt.Fprintln(output, "\nScan errors:")
		for _, issue := range report.Errors {
			fmt.Fprintf(output, "  %s  %s  %s\n", issue.Kind, filepath.ToSlash(issue.Path), issue.Message)
		}
	}
}

type excludeValues []string

func (values *excludeValues) String() string {
	return strings.Join(*values, ",")
}

func (values *excludeValues) Set(value string) error {
	normalized, err := stats.NormalizeExcludePattern(value)
	if err != nil {
		return err
	}
	*values = append(*values, normalized)
	return nil
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	divisor, exponent := int64(unit), 0
	for quotient := size / unit; quotient >= unit && exponent < 3; quotient /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(divisor), "KMGT"[exponent])
}
