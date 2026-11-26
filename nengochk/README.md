# nengochk

nengochk checks Japanese era and Western year pairs such as 令和3年（2021年） in text and reports mismatches.

## Install

```
go install github.com/dorako321/nengochk@latest
```

## Usage

```
nengochk checks Japanese era and Western year pairs such as 令和3年（2021年） in text and reports mismatches.

Usage:
  nengochk [file ...]

Examples:
  echo '平成31年（2020年）' | nengochk
  nengochk a.txt b.txt
  nengochk notes.txt

Flags:
  -version
    	print version and exit
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
