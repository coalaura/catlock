# CatLock

**A small keyboard lock for cats.**

CatLock temporarily intercepts keyboard input before it reaches your other apps. It is useful when a cat wants the keyboard but you do not want to close everything first.

![CatLock](.github/screenshot.png)

## Download

Download the latest executable from [Releases](https://github.com/coalaura/catlock/releases):

- `catlock-windows-amd64.exe` for most Windows computers
- `catlock-windows-arm64.exe` for Windows on ARM
- `catlock-linux-amd64` for most Linux computers
- `catlock-linux-arm64` for Linux on ARM

No installation is required.

The Linux version requires an X11 desktop session. XWayland cannot prevent keyboard input from reaching native Wayland applications.

## Usage

1. Run CatLock to lock the keyboard.
2. Let the paws take over.
3. Select **Release keyboard** or press `Ctrl + Alt + Shift + F12` when you are ready. Select **Release + log** instead to also open the capture file.

Key presses are saved only on your computer under `%LocalAppData%\CatLock\Captures` on Windows, or `$XDG_CACHE_HOME/CatLock/Captures` (normally `~/.cache/CatLock/Captures`) on Linux. The capture file is only opened when releasing through **Release + log**.

## Build

CatLock requires Go 1.27 or newer.

```powershell
go generate ./...
go build -trimpath -ldflags="-H=windowsgui" -o catlock.exe .
```

On Linux:

```sh
go build -trimpath -o catlock .
```
