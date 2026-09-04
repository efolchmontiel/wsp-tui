# WhatsTUI — Development

## Prerequisites

- Go **1.26+** (developed with Go 1.27.1)
- Linux terminal with UTF-8
- Optional later: `mpv`, `xdg-open`

No Node.js. No Chromium. No browser automation.

## Build

```bash
go build -o whatstui ./cmd/whatstui
```

Or:

```bash
make build
```

## Run

```bash
./whatstui
./whatstui --debug
./whatstui --version
./whatstui --logout
./whatstui --reset
```

## Data locations

| Path | Purpose |
|------|---------|
| `~/.local/share/whatstui/session.db` | whatsmeow session |
| `~/.local/share/whatstui/whatstui.db` | app UI database |
| `~/.local/share/whatstui/media/` | media files |
| `~/.local/state/whatstui/whatstui.log` | logs |
| `~/.config/whatstui/config.toml` | config |

## First login

1. Run `whatstui`
2. Scan the QR with WhatsApp → Linked Devices
3. Or press `p` and enter your phone number (international, no `+`) for a pairing code
4. Status becomes Connected; session persists for next launches

## Architecture reminders

- Keep WhatsApp work off the UI loop
- Never write UI tables into `session.db`
- Pin whatsmeow versions intentionally (pre-1.0 API)

## Updating whatsmeow

```bash
go get go.mau.fi/whatsmeow@latest
go mod tidy
```

Re-read godoc before bumping: the public API can change between pseudo-versions.

## Tests (later phases)

```bash
go test ./...
```

## License notes

See [licenses.md](./licenses.md). Do not copy code from other WhatsApp TUI projects without license review.
