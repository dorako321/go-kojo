# csvwrap

csvwrap reads CSV from a file or stdin and prints a ruled table that fits a given display width, wrapping long cells inside their columns.

## Install

```
go install github.com/dorako321/csvwrap@latest
```

## Usage

```
csvwrap reads CSV from a file or stdin and prints a ruled table that fits a given display width, wrapping long cells inside their columns.

Usage:
  csvwrap [-w width] [file]

Examples:
  csvwrap data.csv
  csvwrap -w 60 data.csv
  cat data.csv | csvwrap -w 40

Flags:
  -version
    	print version and exit
  -w int
    	total display width of the table, including borders and padding (default 80)
```

```
$ printf 'id,desc\n1,abcdefghij\n' | csvwrap -w 15
+----+--------+
| id | desc   |
+----+--------+
| 1  | abcdef |
|    | ghij   |
+----+--------+
```

```
$ printf 'k\nあいうえお\n' | csvwrap -w 10
+--------+
| k      |
+--------+
| あいう |
| えお   |
+--------+
```

```
$ printf 'k\nあいう\n' | csvwrap -w 9
+-------+
| k     |
+-------+
| あい  |
| う    |
+-------+
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
