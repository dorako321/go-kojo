# wrapal

wrapal wraps text from a file or stdin to a display width and aligns each line left, right, center or justified.

## Install

```
go install github.com/dorako321/wrapal@latest
```

## Usage

```
wrapal wraps text from a file or stdin to a display width and aligns each line left, right, center or justified.

Usage:
  wrapal [-w width] [-a left|right|center|justify] [file]

Examples:
  wrapal -w 30 notes.txt
  wrapal -w 40 -a justify < article.txt
  echo "hello world" | wrapal -w 20 -a center

Flags:
  -a string
    	alignment: left, right, center or justify (default "left")
  -version
    	print version and exit
  -w int
    	target display width (full-width chars count 2) (default 40)
```

```
$ echo 'the quick brown fox jumps' | wrapal -w 10
the quick
brown fox
jumps
```

```
$ echo abc | wrapal -w 10 -a center
   abc    
```

```
$ printf 'ab\n\n\n\ncd\n' | wrapal -w 5 -a right
   ab

   cd
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
