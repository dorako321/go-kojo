# zenhan

zenhan normalizes Japanese text by converting full-width ASCII to half-width and half-width katakana to full-width katakana, or lists the lines needing conversion with -r.

## Install

```
go install github.com/dorako321/zenhan@latest
```

## Usage

```
zenhan normalizes Japanese text by converting full-width ASCII to half-width and half-width katakana to full-width katakana, or lists the lines needing conversion with -r.

Usage:
  zenhan [-r] [file ...]

Examples:
  echo 'ＡＢＣ１２３ ｶﾞｷﾞ' | zenhan
  zenhan notes.txt > normalized.txt
  zenhan -r a.txt b.txt

Flags:
  -r	report lines that would change as 'lineNumber: original line' instead of converting
  -version
    	print version and exit
```

```
$ echo 'ＡＢＣ１２３ ｶﾞｷﾞｸﾞ' | zenhan
ABC123 ガギグ
```

```
$ printf 'hello\nＡＢＣ\n日本語\nｱｲｳ\n' | zenhan -r
2: ＡＢＣ
4: ｱｲｳ
```

```
$ echo 'ｱﾞ ﾞ' | zenhan
ア゛ ゛
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
