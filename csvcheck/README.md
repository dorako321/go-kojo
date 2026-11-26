# csvcheck

csvcheck reads CSV from a file or stdin and reports structural problems line by line, exiting 1 if any are found.

## Install

```
go install github.com/dorako321/csvcheck@latest
```

## Usage

```
csvcheck reads CSV from a file or stdin and reports structural problems line by line, exiting 1 if any are found.

Usage:
  csvcheck [-k cols] [file]

Examples:
  csvcheck data.csv
  csvcheck -k 1,2 data.csv
  cat data.csv | csvcheck

Flags:
  -k string
    	comma-separated 1-based column numbers used for duplicate row detection (default: whole row)
  -version
    	print version and exit
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
