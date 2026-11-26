# mdid

mdid reads CSV from a file or stdin and prints a Markdown table with a deterministic UUID v5 id column computed from each row's content.

## Install

```
go install github.com/dorako321/mdid@latest
```

## Usage

```
mdid reads CSV from a file or stdin and prints a Markdown table with a deterministic UUID v5 id column computed from each row's content.

Usage:
  mdid [-s] [file]

Examples:
  mdid data.csv
  mdid -s data.csv
  cat data.csv | mdid -s

Flags:
  -s	shorten each id to its first 8 characters
  -version
    	print version and exit
```

```
$ printf 'name,qty\nりんご,3\n' | mdid
| id | name | qty |
| --- | --- | --- |
| 00e12882-17a4-5db4-a593-cefd8daf0fcf | りんご | 3 |
```

```
$ printf 'name,qty\nりんご,3\n' | mdid -s
| id | name | qty |
| --- | --- | --- |
| 00e12882 | りんご | 3 |
```

```
$ printf 'a,b\n1\n' | mdid -s
| id | a | b |
| --- | --- | --- |
| 1e7b2703 | 1 |  |
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
