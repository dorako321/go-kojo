# ipfold

ipfold extracts unique IPv4 addresses from text and prints them grouped by prefix, wrapped to a given width.

## Install

```
go install github.com/dorako321/ipfold@latest
```

## Usage

```
ipfold extracts unique IPv4 addresses from text and prints them grouped by prefix, wrapped to a given width.

Usage:
  ipfold [-p 8|16|24] [-w width] [file ...]

Examples:
  echo "10.0.1.5 10.0.1.33 10.0.2.7" | ipfold
  ipfold -p 16 -w 60 access.log
  ipfold -p 8 a.txt b.txt

Flags:
  -p int
    	prefix length: 8, 16 or 24 (default 24)
  -version
    	print version and exit
  -w int
    	display width for wrapping (must be > 0) (default 80)
```

```
$ echo 'log 10.0.1.5 and 10.0.1.33, then 10.0.2.7 and 10.0.1.12' | ipfold
10.0.1.: 5 12 33
10.0.2.: 7
```

```
$ echo '10.0.1.5 10.0.2.7 10.0.1.5 10.1.0.1' | ipfold -p 16
10.0.: 1.5 2.7
10.1.: 0.1
```

```
$ echo '10.0.1.5 10.0.0.1 11.2.3.4' | ipfold -p 8
10.: 0.0.1 0.1.5
11.: 2.3.4
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
