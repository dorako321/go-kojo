# hashart

hashart prints a colored 17x9 drunken-bishop randomart and the hex SHA-256 of a file or stdin.

## Install

```
go install github.com/dorako321/hashart@latest
```

## Usage

```
hashart prints a colored 17x9 drunken-bishop randomart and the hex SHA-256 of a file or stdin.

Usage:
  hashart [-n] [file]

Examples:
  hashart photo.jpg
  hashart -n notes.txt
  echo hello | hashart -n

Flags:
  -n	no color: print plain ASCII without escape codes
  -version
    	print version and exit
```

```
$ hashart -n
+-----------------+
| ++=E.           |
|+=B*             |
|.=@oo            |
|.B.O o           |
|  X = + S        |
| . = B + .       |
|  . O =..        |
|   o *o          |
|  ....o.         |
+-----------------+
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
