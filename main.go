package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ramiabukhader/chunkstat/internal/stats"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("chunkstat", flag.ContinueOnError)
	flags.SetOutput(stderr)
	top := flags.Int("top", 10, "number of largest files to display")
	asJSON := flags.Bool("json", false, "print the report as JSON")
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

	report, err := stats.Scan(root, *top)
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
		return 0
	}

	printReport(stdout, report)
	return 0
}

func printReport(output io.Writer, report stats.Report) {
	fmt.Fprintf(output, "Directory:       %s\n", report.Root)
	fmt.Fprintf(output, "Files:           %d\n", report.Files)
	fmt.Fprintf(output, "Total lines:     %d\n", report.TotalLines)
	fmt.Fprintf(output, "Ignored folders: %d\n", len(report.IgnoredFolders))

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
