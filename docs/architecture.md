# WhatsTUI — Architecture

## Decision summary

**WhatsTUI Phase 1+ uses a monolithic Go process** with clear internal packages:

- WhatsApp protocol: `go.mau.fi/whatsmeow`
- TUI: Charm Bubble Tea + Lip Gloss
- Local app state: SQLite (`whatstui.db`)
- Session crypto/store: SQLite managed by whatsmeow (`session.db`)

We evaluated the preferred split (`Go backend` + `Rust/Ratatui frontend`) and **deferred it**.

## Why not Go + Rust/Ratatui yet?

| Criterion | Monolith Go | Go engine + Rust TUI |
|-----------|-------------|----------------------|
| Performance (UI lag) | Excellent if event-driven | Excellent *if* IPC is non-blocking |
| Stability | One runtime, one binary | Dual process / dual toolchain |
| Maintainability | One language, whatsmeow native | IPC contracts, dual deps, dual CI |
| Multimedia | Native whatsmeow Upload/Download | Extra serialization + file handoff |
| Responsiveness | Channels + Bubble Tea loop | Same *plus* IPC latency/failure modes |

The bottleneck in slow WhatsApp TUIs is almost never the TUI crate. It is:

1. blocking the UI on network/SQLite/media
2. eager full-history sync on startup
3. re-rendering too much state per keystroke

Crossing the language boundary for Phase 1 would add complexity **without** fixing those problems. Ratatui remains a valid future frontend if we publish a stable local protocol (Unix socket / JSON lines / gRPC) once the engine API settles.

## Runtime topology

```text
┌────────────────────────────────────────────────────────────┐
│                     whatstui (single process)              │
│                                                            │
│  ┌─────────────┐   events    ┌──────────────────────────┐  │
│  │ Bubble Tea  │ ◄────────── │ app.EventBus             │  │
│  │ UI loop     │ ──────────► │ (non-blocking fan-out)   │  │
│  └─────────────┘  commands   └────────────▲─────────────┘  │
│         │                                 │                │
│         │ never awaits WA/SQLite          │                │
│         ▼                                 │                │
│  ┌─────────────┐                 ┌────────┴─────────┐      │
│  │ UI state    │ ◄── reads ──────│ App SQLite       │      │
│  │ (in-memory) │                 │ whatstui.db      │      │
│  └─────────────┘                 └────────▲─────────┘      │
│                                           │ writes         │
│                                  ┌────────┴─────────┐      │
│                                  │ Sync / Engine    │      │
│                                  │ (goroutines)     │      │
│                                  └────────▲─────────┘      │
│                                           │                │
│                                  ┌────────┴─────────┐      │
│                                  │ whatsmeow Client │      │
│                                  │ session.db       │      │
│                                  └──────────────────┘      │
└────────────────────────────────────────────────────────────┘
```

**Hard rule:** the Bubble Tea update/view path never calls WhatsApp, network, or heavy SQLite. Those jobs run in engine/sync goroutines and publish events.

## Package layout

```text
cmd/whatstui/          CLI entrypoint
internal/app/          orchestration, event bus, lifecycle
internal/engine/       whatsmeow client, auth, reconnect
internal/store/        application SQLite (UI cache)
internal/paths/        XDG data/config/state directories
internal/logging/      file-only logging (never pollutes TUI)
internal/ui/           Bubble Tea models/views
internal/config/       config.toml loader (minimal for now)
internal/version/      version string
```

## Data directories (XDG)

```text
~/.local/share/whatstui/
├── session.db          # whatsmeow device/session store ONLY
├── whatstui.db         # application UI cache
├── media/
│   ├── images/
│   ├── videos/
│   ├── audio/
│   ├── documents/
│   └── stickers/
└── cache/

~/.local/state/whatstui/
└── whatstui.log

~/.config/whatstui/
└── config.toml
```

Session tables and UI tables are never mixed.

## Auth (current whatsmeow API)

Pinned module (Phase 1 start): `go.mau.fi/whatsmeow v0.0.0-20260903111606-de26b4ab6499`.

Relevant APIs verified against that revision:

- `sqlstore.New(ctx, dialect, address, log)`
- `container.GetFirstDevice(ctx)`
- `whatsmeow.NewClient(deviceStore, log)`
- `client.GetQRChannel(ctx)` → `QRChannelItem{Event, Code, ...}`
- `client.Connect()` / `ConnectContext(ctx)`
- `client.PairPhone(ctx, phone, showPush, clientType, displayName)`
- `client.AddEventHandler(handler)`
- `client.EnableAutoReconnect` (default true; built-in backoff)
- Events: `*events.Connected`, `*events.Disconnected`, `*events.LoggedOut`, `*events.PairSuccess`, `*events.QR`

SQLite driver: `modernc.org/sqlite` (pure Go) with dialect `"sqlite"` (accepted by `go.mau.fi/util/dbutil.ParseDialect` because it prefixes `sqlite`).

## Event model (app-level)

Engine events are mapped to app events before they reach the UI:

- `Connected` / `Disconnected` / `Reconnecting`
- `QRCode` / `PairingCode` / `PairSuccess`
- `LoggedOut`
- later phases: message/chat/media/sync events

## Non-goals for Phase 1

- Chat list, messages, media, search, themes beyond a dark shell
- Cross-language IPC
- Browser automation of any kind

## Priority order (always)

1. Stability
2. Responsiveness
3. Performance
4. Functionality
5. Aesthetics
