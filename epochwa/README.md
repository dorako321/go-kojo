# epochwa

epochwa copies text from files or stdin and appends a Japanese-era date-time in full-width parentheses after every 10-digit or 13-digit UNIX timestamp.

## Install

```
go install github.com/dorako321/epochwa@latest
```

## Usage

```
epochwa copies text from files or stdin and appends a Japanese-era date-time in full-width parentheses after every 10-digit or 13-digit UNIX timestamp.

Usage:
  epochwa [-z hours] [file ...]

Examples:
  echo "ts=1735689600" | epochwa
  epochwa -z 0 access.log
  epochwa -z 5.5 a.txt b.txt

Flags:
  -version
    	print version and exit
  -z string
    	timezone offset from UTC in hours (-12 to 14, fractions allowed) (default "9")
```

```
$ echo ts=1735689600 | epochwa
ts=1735689600（令和7年1月1日 9:00:00）
```

```
$ printf 't=1735689600000 end' | epochwa
t=1735689600000（令和7年1月1日 9:00:00） end
```

```
$ echo 'a 12345678901 b 123456789012 c 12345678901234' | epochwa
a 12345678901 b 123456789012 c 12345678901234
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
