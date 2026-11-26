# mdbar

mdbar reads CSV from a file or stdin and prints a Markdown table with an extra bar column for one numeric column.

## Install

```
go install github.com/dorako321/mdbar@latest
```

## Usage

```
mdbar reads CSV from a file or stdin and prints a Markdown table with an extra bar column for one numeric column.

Usage:
  mdbar -c column [-w width] [file]

Examples:
  mdbar -c sales data.csv
  mdbar -c 2 -w 10 data.csv
  cat data.csv | mdbar -c qty -w 30

Flags:
  -c string
    	column to graph: header name or 1-based number (required)
  -version
    	print version and exit
  -w int
    	bar width for the largest value (1 or more) (default 20)
```

```
$ printf 'name,sales\n東京,30\n大阪,15\n' | mdbar -c sales -w 4
| name | sales | bar  |
| ---- | ----- | ---- |
| 東京 | 30    | ████ |
| 大阪 | 15    | ██   |
```

```
$ printf 'item,qty\na,5\nb,x\nc,10\n' | mdbar -c 2 -w 10
| item | qty | bar        |
| ---- | --- | ---------- |
| a    | 5   | █████      |
| b    | x   |            |
| c    | 10  | ██████████ |
```

```
$ printf 'a,v\nx,0\n' | mdbar -c v -w 3
| a   | v   | bar |
| --- | --- | --- |
| x   | 0   |     |
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
