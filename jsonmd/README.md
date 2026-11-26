# jsonmd

jsonmd reads a JSON array of objects or NDJSON from a file or stdin and prints a Markdown table whose columns are the union of all keys.

## Install

```
go install github.com/dorako321/jsonmd@latest
```

## Usage

```
jsonmd reads a JSON array of objects or NDJSON from a file or stdin and prints a Markdown table whose columns are the union of all keys.

Usage:
  jsonmd [file]

Examples:
  jsonmd data.json
  jsonmd data.jsonl > table.md
  curl -s example.com/items | jsonmd

Flags:
  -version
    	print version and exit
```

```
$ echo '[{"name":"a","n":1},{"name":"b","tag":[1,2]}]' | jsonmd
| name | n | tag |
| --- | --- | --- |
| a | 1 |  |
| b |  | [1,2] |
```

```
$ printf '[{"a":"l1\\nl2"}]' | jsonmd
| a |
| --- |
| l1 l2 |
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
