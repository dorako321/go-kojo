# colhash

colhash prints a Markdown table of per-column SHA-256 prefixes for one CSV file, or compares them between two CSV files.

## Install

```
go install github.com/dorako321/colhash@latest
```

## Usage

```
colhash prints a Markdown table of per-column SHA-256 prefixes for one CSV file, or compares them between two CSV files.

Usage:
  colhash [-n digits] file1.csv [file2.csv]

Examples:
  colhash data.csv
  colhash -n 12 old.csv new.csv

Flags:
  -n int
    	number of hex characters of each hash (1 to 64) (default 8)
  -version
    	print version and exit
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
