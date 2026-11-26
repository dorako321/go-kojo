# youbichk

youbichk finds dates with a written weekday in Japanese text and reports those whose weekday does not match the real calendar.

## Install

```
go install github.com/dorako321/youbichk@latest
```

## Usage

```
youbichk finds dates with a written weekday in Japanese text and reports those whose weekday does not match the real calendar.

Usage:
  youbichk [-a] [file ...]

Examples:
  youbichk memo.txt
  youbichk -a a.txt b.txt
  echo '令和3年4月5日（火）' | youbichk

Flags:
  -a	also print dates whose weekday matches
  -version
    	print version and exit
```

```
$ echo '2021年4月5日(月)' | youbichk -a
1: 2021年4月5日 は月曜日（一致）
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
