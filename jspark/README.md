# jspark

jspark reads a JSON array or NDJSON from a file or stdin, extracts numbers (optionally at a dotted key path) and prints a one-line sparkline with min, max and count.

## Install

```
go install github.com/dorako321/jspark@latest
```

## Usage

```
jspark reads a JSON array or NDJSON from a file or stdin, extracts numbers (optionally at a dotted key path) and prints a one-line sparkline with min, max and count.

Usage:
  jspark [-k key] [file]

Examples:
  echo '[3,1,4,1,5,9,2,6]' | jspark
  jspark -k ms timings.ndjson
  jspark -k a.b data.json

Flags:
  -k string
    	key path of the number in each element, dot-separated for nesting (e.g. a.b)
  -version
    	print version and exit
```

```
$ printf '[{"ms":120},{"ms":340},{"ms":90}]' | jspark -k ms
▂█▁  min=90 max=340 n=3
```

```
$ printf '[1,2,3,4,5,6,7,8]' | jspark
▁▂▃▄▅▆▇█  min=1 max=8 n=8
```

```
$ printf '{"a":{"b":0}}\n{"a":{"c":1}}\n\n{"a":{"b":7}}\n5\n' | jspark -k a.b
▁█  min=0 max=7 n=2
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
