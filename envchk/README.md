# envchk

envchk checks a .env file (or stdin) line by line for invalid keys, duplicate keys, trailing spaces, full-width characters and unclosed quotes.

## Install

```
go install github.com/dorako321/envchk@latest
```

## Usage

```
envchk checks a .env file (or stdin) line by line for invalid keys, duplicate keys, trailing spaces, full-width characters and unclosed quotes.

Usage:
  envchk [file]

Examples:
  envchk .env
  envchk prod.env
  cat .env | envchk

Flags:
  -version
    	print version and exit
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
