package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ramiabukhader/chunkstat/internal/stats"
)

func main() {
	os.Exit(run())
}

func run() int {
	top := flag.Int("top", 10, "number of largest files to display")
	asJSON := flag.Bool("json", false, "print the report as JSON")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: chunkstat [flags] [directory]\n\n")
		fmt.Fprintln(flag.CommandLine.Output(), "Scan a directory and summarize file and line counts.")
		fmt.Fprintln(flag.CommandLine.Output(), "\nFlags:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *top < 0 {
		fmt.Fprintln(os.Stderr, "chunkstat: -top must be zero or greater")
		return 2
	}
	if flag.NArg() > 1 {
		flag.Usage()
		return 2
	}

	root := "."
	if flag.NArg() == 1 {
		root = flag.Arg(0)
	}

	report, err := stats.Scan(root, *top)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chunkstat: %v\n", err)
		return 1
	}

	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "chunkstat: write report: %v\n", err)
			return 1
		}
		return 0
	}

	printReport(report)
	return 0
}

func printReport(report stats.Report) {
	fmt.Printf("Directory:       %s\n", report.Root)
	fmt.Printf("Files:           %d\n", report.Files)
	fmt.Printf("Total lines:     %d\n", report.TotalLines)
	fmt.Printf("Ignored folders: %d\n", len(report.IgnoredFolders))

	fmt.Println("\nBy extension:")
	fmt.Printf("  %-16s %10s %12s\n", "EXTENSION", "FILES", "LINES")
	for _, extension := range report.ByExtension {
		fmt.Printf("  %-16s %10d %12d\n", extension.Extension, extension.Files, extension.Lines)
	}

	if len(report.LargestFiles) > 0 {
		fmt.Println("\nLargest files:")
		fmt.Printf("  %-12s %12s  %s\n", "SIZE", "LINES", "PATH")
		for _, file := range report.LargestFiles {
			fmt.Printf("  %-12s %12d  %s\n", formatBytes(file.Bytes), file.Lines, filepath.ToSlash(file.Path))
		}
	}

	if len(report.IgnoredFolders) > 0 {
		fmt.Println("\nIgnored folder paths:")
		for _, path := range report.IgnoredFolders {
			fmt.Printf("  %s\n", filepath.ToSlash(path))
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
