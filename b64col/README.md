# b64col

b64col reads CSV from a file or stdin and Base64-decodes the cells in the chosen columns, writing CSV to stdout.

## Install

```
go install github.com/dorako321/b64col@latest
```

## Usage

```
b64col reads CSV from a file or stdin and Base64-decodes the cells in the chosen columns, writing CSV to stdout.

Usage:
  b64col [-u] -c cols [file]

Examples:
  b64col -c 2 in.csv
  b64col -u -c 1,3 in.csv
  cat in.csv | b64col -c 2

Flags:
  -c string
    	comma-separated 1-based column numbers to decode (required)
  -u	also accept URL-safe Base64 (- and _)
  -version
    	print version and exit
```

```
$ echo 1,SGVsbG8= | b64col -c 2
1,Hello
```

```
$ echo a,SGk_ | b64col -u -c 2
a,Hi?
```

```
$ echo a,SGk_ | b64col -c 2
a,SGk_
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
