# chunkstat

`chunkstat` is a small, fast command-line tool that summarizes the contents of a directory. It reports file and line totals, groups counts by extension, highlights the largest files, and skips common dependency and build folders.

## Features

- Counts regular files and text lines recursively
- Shows file counts and line counts by extension
- Lists the largest files in the scan
- Ignores `.git`, `node_modules`, `bin`, `obj`, and `vendor` directories
- Supports human-readable terminal output and JSON
- Treats extension names case-insensitively
- Counts binary files in file totals while excluding them from line totals

## Install

With Go 1.22 or later:

```sh
go install github.com/ramiabukhader/chunkstat@latest
```

Or clone the repository and build it locally:

```sh
git clone https://github.com/ramiabukhader/chunkstat.git
cd chunkstat
go build .
```

## Usage

Scan the current directory:

```sh
chunkstat
```

Scan another directory and show the five largest files:

```sh
chunkstat -top 5 ../my-project
```

Produce machine-readable output:

```sh
chunkstat -json .
```

Exclude repository-relative files or directory trees with repeatable patterns:

```sh
chunkstat --exclude generated --exclude "*.tmp" .
```

Patterns use slash-normalized Go glob syntax on every operating system. A plain
directory path excludes its entire tree. Absolute paths, parent traversal, and
empty patterns are rejected before scanning.

Example terminal report:

```text
Directory:       /home/user/project
Files:           42
Total lines:     3860
Ignored folders: 2
Excluded paths:  0

By extension:
  EXTENSION             FILES        LINES
  .go                      18         2140
  .md                       4          310
  .yaml                     3          120

Largest files:
  SIZE                LINES  PATH
  24.7 KiB              640  internal/report/report.go
  18.1 KiB              482  cmd/server/main.go
```

Run `chunkstat -help` for all available flags.

## Development

Format and validate the project with:

```sh
gofmt -w .
go test -race ./...
go vet ./...
go build ./...
```

GitHub Actions runs the same checks for every pull request and push to `main`.

## Notes

`chunkstat` does not follow directory symlinks. A file containing a NUL byte is treated as binary: it is included in file and size statistics but contributes zero lines.

## License

This project is available under the [MIT License](LICENSE).
