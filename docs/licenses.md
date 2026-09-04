# WhatsTUI — License review (dependencies)

Provisional project license: **MIT** (application code we author).

This is not legal advice; it is an engineering checklist of obligations we must respect.

## Critical dependency: whatsmeow

| Item | Value |
|------|-------|
| Package | `go.mau.fi/whatsmeow` |
| License | **MPL-2.0** with *Incompatible With Secondary Licenses* |
| Implication | File-level copyleft on whatsmeow source we modify/distribute |
| Our approach | Link as a dependency; do not vendor-modify unless necessary. If we patch whatsmeow files, those files stay MPL-2.0 and we ship their notices. |

We can keep WhatsTUI itself under MIT. We must **not** relicense whatsmeow under GPL-style secondary terms.

## TUI stack (Phase 1)

| Package | License (typical) | Notes |
|---------|-------------------|-------|
| `github.com/charmbracelet/bubbletea` | MIT | UI framework |
| `github.com/charmbracelet/lipgloss` | MIT | Styling |
| `github.com/charmbracelet/bubbles` | MIT | Components |
| `github.com/skip2/go-qrcode` | MIT | QR encoding |

## Deferred preference: Ratatui

| Package | License |
|---------|---------|
| `ratatui` (Rust) | MIT |

Not linked in Phase 1 (see architecture decision). Safe to adopt later under MIT.

## SQLite

| Package | License |
|---------|---------|
| `modernc.org/sqlite` | BSD-3-Clause (verify on upgrade) |

## Policy

1. Do not copy substantial code from WhatsCLI, wstui, or other clients without reading their licenses first.
2. Keep third-party notices available when distributing binaries.
3. Never log private keys / session secrets.
4. `session.db` permissions should stay user-private (`0700` dirs, `0600` files).
