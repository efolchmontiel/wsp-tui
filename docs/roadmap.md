# WhatsTUI — Roadmap

## Status

| Phase | Goal | Status |
|-------|------|--------|
| 1 | Bootstrap: build, SQLite, whatsmeow, QR, reconnect | **Done** |
| 2 | Chats + text messages | **Done** |
| 3 | Performance: cache, pagination, sync, render | **Done** |
| 4 | Multimedia | **Done** |
| 5 | UX: search, mouse, themes, shortcuts | **Done** |
| 6 | Robustness: tests, benchmarks, profiling | Not started |

## Phase 1 — Bootstrap (current)

Deliverable: `whatstui` → QR (or existing session) → Connected, without freezing.

- [x] Project layout + architecture docs
- [x] XDG paths (`share` / `state` / `config`)
- [x] File logging (`--debug`)
- [x] App SQLite stub (`whatstui.db`) separate from `session.db`
- [x] whatsmeow client wrapper
- [x] QR login in TUI
- [x] Session reuse on restart
- [x] Pairing code path (`PairPhone`)
- [x] Reconnect UX (`Connected` / `Reconnecting`)
- [x] CLI: `--version`, `--debug`, `--logout`, `--reset`

Exit criteria: binary compiles; first run shows QR; second run reconnects without QR unless logged out.

**Verified:** `go build ./cmd/whatstui`, `go test ./...`, engine dials `wss://web.whatsapp.com` in background. Manual QR scan requires a real TTY.

## Phase 2 — Chats (text)

- [x] Chat list from local DB first
- [x] HistorySync → app SQLite (background syncer)
- [x] Live Message / Receipt events
- [x] Message receive/send text
- [x] Optimistic send statuses (sending → sent → delivered/read)
- [x] Keyboard navigation between sidebar / transcript / input
- [x] Load last 50 messages; PageUp loads older

Exit criteria: after login, chats appear from sync; can open a chat, read text, send text without UI freezes.

## Phase 3 — Performance

- [x] Debounced sidebar reload (120ms coalesce)
- [x] In-memory chat preview patch on live messages
- [x] Message pagination (Phase 2)
- [x] Background syncer (Phase 2)
- [x] Live messages no longer force full `ChatsDirty` every time
- [x] Group subjects via `GetJoinedGroups` + `events.GroupInfo`
- [x] Status broadcast labeled `Estados` (not a fake group)
- [x] Sidebar virtual window (`sidebarOff`) for large lists
- [ ] Benchmarks harness (Phase 6)

## Phase 3 note

Input path never awaits SQLite: keystrokes update `textinput` only. Sidebar DB refresh is coalesced so sync bursts cannot stall typing. Group titles come from WhatsApp group metadata, never from a participant push name.

## Phase 4 — Multimedia

- [x] Upload/download via whatsmeow (`Upload` / `UploadReader` / `DownloadToFile`)
- [x] Filesystem media store under `~/.local/share/whatstui/media/`
- [x] Persist download refs in `metadata_json` + `media` table
- [x] Auto-download small images (≤512 KiB)
- [x] External open (`mpv` for a/v, `xdg-open` otherwise)
- [x] File picker (`Ctrl+O`); open (`o`) / download (`d`)
- [x] Non-blocking upload progress via EventBus

Exit criteria: can attach a file, receive media, download on demand, open externally without freezing the input.

## Phase 5 — UX

- [x] Search (FTS5) over messages + LIKE on chat names (`/` / `Ctrl+F`)
- [x] Mouse: click chats, focus panes, wheel scroll (`mouse = true` in config)
- [x] Themes: dark / light / ocean / forest (`t` cycles; persisted in `config.toml`)
- [x] Error modal + help modal (`?`)
- [x] Shortcuts finalized in help overlay + README
- [x] Sidebar filters: Todos / Favoritos / Grupos / Archivados (`1`–`4`)
- [x] Archivar chat (`e`) + favorito (`f`)
- [x] Desktop notify (`notify-send`) for messages in other chats
- [x] Media cursor (`[` / `]`) + ASCII wave while audio plays
- [x] Local retention purge: messages + media older than 90 days on startup (favorites/archived included; phone untouched)
- [x] Novedades filter for communities (excluded from Todos/Grupos)
- [x] Blue read receipts (✓✓) when peer has read receipts enabled
- [x] Disappearing messages cycle (`m`: Off/24h/7d/90d) for DMs and groups
- [x] Incoming/missed call banners in transcript (yellow / light red); no answer from TUI

## Phase 6 — Robustness

- Unit + integration tests
- Benchmarks under `benchmarks/`
- Profiling under sync load (input lag must stay near zero)

## Explicit non-goals forever

- Chromium / Chrome / Selenium / Puppeteer / Playwright
- whatsapp-web.js / Node browser automation
- Forking WhatsCLI or wstui
