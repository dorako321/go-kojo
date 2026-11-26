# u4scan

u4scan reports characters that break in old MySQL utf8 or Shift_JIS conversion (4-byte UTF-8 characters, variation selectors, zero width joiner) with line and column.

## Install

```
go install github.com/dorako321/u4scan@latest
```

## Usage

```
u4scan reports characters that break in old MySQL utf8 or Shift_JIS conversion (4-byte UTF-8 characters, variation selectors, zero width joiner) with line and column.

Usage:
  u4scan [-c] [file ...]

Examples:
  u4scan names.txt
  u4scan -c a.csv b.csv
  cat dump.sql | u4scan

Flags:
  -c	CSV mode: also print the 1-based CSV column number of each finding
  -version
    	print version and exit
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
