# rainbowqr

rainbowqr shows the given text as a QR code in the terminal with a rainbow color animation.

## Install

```
go install github.com/dorako321/rainbowqr@latest
```

## Usage

```
rainbowqr shows the given text as a QR code in the terminal with a rainbow color animation.

Usage:
  rainbowqr [-d seconds] [text]

Examples:
  rainbowqr https://example.com
  rainbowqr -d 5 "hello world"
  echo hello | rainbowqr -d 0

Flags:
  -d float
    	animation length in seconds (0 disables the animation) (default 3)
  -version
    	print version and exit
```

```
$ rainbowqr hello
█████████████████████████████
█████████████████████████████
████ ▄▄▄▄▄ ██▀█▀▀█ ▄▄▄▄▄ ████
████ █   █ █ █ ▄ █ █   █ ████
████ █▄▄▄█ █ █▄▀▄█ █▄▄▄█ ████
████▄▄▄▄▄▄▄█ █ █▄█▄▄▄▄▄▄▄████
████ ▀ ▄ ▄▄▀██  ▀▀ ▄▄  █▀████
█████▄▄█ ▀▄▀███▀ ▀ ▄█  ▄█████
█████▄█▄██▄▄  ▀█ ██ █▀██ ████
████ ▄▄▄▄▄ █▀▄▀▄█▄█▀ ▀  █████
████ █   █ █  █ ▀ ▀▀▄▀█▄█████
████ █▄▄▄█ █▄▀█▀ ▀ ▄▀▀ ██████
████▄▄▄▄▄▄▄█▄███▄███▄██▄█████
█████████████████████████████
▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
```

```
$ echo https://example.com | rainbowqr
█████████████████████████████████
█████████████████████████████████
████ ▄▄▄▄▄ ███▀▄▄ ▀▀ █ ▄▄▄▄▄ ████
████ █   █ █ ▄█▄█▀ ▀██ █   █ ████
████ █▄▄▄█ █ ▄▄▀█▄▀▀▄█ █▄▄▄█ ████
████▄▄▄▄▄▄▄█ █▄█▄█ █ █▄▄▄▄▄▄▄████
████▄▀▄▄  ▄█▀█▀▀▄▀███▀▄ ▄▄▄▀█████
████  ▄  ▀▄█ ▄▀▀█ ▄  █▀ █▄█▄ ████
████▀   ▀█▄█▀█▀▄▄█▄▄▀▄▄▀▄▀▄ ▄████
████ ██▀██▄ ██▄ ▄█▀ ▀▀   ▄█▄ ████
████▄█▄██▄▄█  ▄▄▀▀▀▀ ▄▄▄ ▀▄██████
████ ▄▄▄▄▄ █▀▀██▀ ▄█ █▄█ ▀▄ ▄████
████ █   █ █ ▄▄█▄█▄ ▄  ▄ ▀ ▀ ████
████ █▄▄▄█ █▄▄▄ ▄█▀▄▀▀█▀▀ ▄█ ████
████▄▄▄▄▄▄▄█▄▄█▄█████▄▄▄▄▄▄▄▄████
█████████████████████████████████
▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
```

```
$ rainbowqr -d 0 abc
█████████████████████████████
█████████████████████████████
████ ▄▄▄▄▄ ██▀ ▄▄█ ▄▄▄▄▄ ████
████ █   █ █ ▄▄▀▀█ █   █ ████
████ █▄▄▄█ █ █ ▀▄█ █▄▄▄█ ████
████▄▄▄▄▄▄▄█ ▀▄█▄█▄▄▄▄▄▄▄████
████ ▀ ▄▄ ▄▀▀▄▄ ▀▀ ▄▄▄ █▀████
█████▄██▀█▄▄▀▄▀▀ ▀ ▄█▀▀ █████
█████▄██▄▄▄█ █▄█ ██ ██▀█▀████
████ ▄▄▄▄▄ █▀▀▄▄█▄█▀ ▄▀ ▀████
████ █   █ █  █ ▀ ▀█▄▀▄██████
████ █▄▄▄█ █▄██▀ ▀ ▄█▀███████
████▄▄▄▄▄▄▄█▄▄▄█▄██▄██▄▄█████
█████████████████████████████
▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
```

## License

MIT

---

Built by [go-kojo](https://github.com/dorako321/go-kojo). The request, the spec and the acceptance cases this program was inspected against are in `kojo.json`.
