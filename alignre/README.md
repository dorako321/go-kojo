# alignre

alignre pads lines with spaces so the first match of a regex starts at the same display column on every line.

## Install

```
go install github.com/dorako321/alignre@latest
```

## Usage

```
alignre pads lines with spaces so the first match of a regex starts at the same display column on every line.

Usage:
  alignre -e REGEX [-p N] [file...]

Examples:
  alignre -e = config.txt
  alignre -e '#' -p 2 main.go
  printf 'a=1\nlong=2\n' | alignre -e =

Flags:
  -e string
    	regular expression (Go syntax) to align on (required)
  -p int
    	minimum number of spaces inserted before the match
  -version
    	print version and exit
```

```
$ printf 'a=1\nlong名=2\n' | alignre -e =
a     =1
long名=2
```

```
$ printf 'a=1\nlong名=2\n' | alignre -e = -p 2
a       =1
long名  =2
```

```
$ printf 'ab=1\nnone\nc=2\n' | alignre -e =
ab=1
none
c =2
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
