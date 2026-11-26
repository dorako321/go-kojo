# yenfmt

yenfmt rewrites yen amounts found in text into comma, man or kanji notation and leaves all other text unchanged.

## Install

```
go install github.com/dorako321/yenfmt@latest
```

## Usage

```
yenfmt rewrites yen amounts found in text into comma, man or kanji notation and leaves all other text unchanged.

Usage:
  yenfmt [-s comma|man|kanji] [file ...]

Examples:
  echo '八万五千円' | yenfmt
  echo '1億2345万円' | yenfmt -s man
  yenfmt -s kanji prices.txt

Flags:
  -s string
    	output style: comma (12,000円), man (1億2,345万円) or kanji (一万二千円) (default "comma")
  -version
    	print version and exit
```

```
$ echo 家賃は八万五千円、敷金は¥170000 | yenfmt -s comma
家賃は85,000円、敷金は170,000円
```

```
$ echo 1億2345万円と3万5千円と8500円 | yenfmt -s man
1億2,345万円と3万5,000円と8,500円
```

```
$ echo '12000円 1000円 170000円 100000000円' | yenfmt -s kanji
一万二千円 千円 十七万円 一億円
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
