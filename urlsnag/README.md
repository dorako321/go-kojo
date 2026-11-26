# urlsnag

urlsnag finds http:// and https:// URLs in text and reports those with unencoded non-ASCII characters or stray trailing punctuation, with line:column and a suggested fix.

## Install

```
go install github.com/dorako321/urlsnag@latest
```

## Usage

```
urlsnag finds http:// and https:// URLs in text and reports those with unencoded non-ASCII characters or stray trailing punctuation, with line:column and a suggested fix.

Usage:
  urlsnag [file ...]

Examples:
  urlsnag notes.txt
  urlsnag a.md b.md
  echo 'see https://example.com/資料。' | urlsnag

Flags:
  -version
    	print version and exit
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
