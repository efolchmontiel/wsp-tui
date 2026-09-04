# WhatsTUI — Performance Contract

## North star

> The UI must never block on WhatsApp, network, SQLite, or multimedia.

If choosing between simpler code and slightly more complex non-blocking code, **prefer non-blocking**.

## Forbidden startup pattern

```text
startup → download all chats → download all history → process → render
```

## Required startup pattern

```text
open local SQLite
  → paint UI immediately (even if empty / login)
  → connect WhatsApp in background
  → sync incrementally
```

## Latency budgets (targets)

These are product targets; Phase 1 only establishes the wiring.

| Action | Target feel |
|--------|-------------|
| Cold start to interactive UI | < 100 ms after process start (local only) |
| Keystroke → input paint | next frame; never awaits I/O |
| Switch conversation | < 16 ms to swap in-memory window |
| Open chat (last 50 msgs cached) | < 50 ms |
| Send text | optimistic bubble immediate; network async |
| Heavy sync in background | input lag still ≈ 0 |

## Rules

1. **UI reads local state first.** WhatsApp is a writer to the cache, not a synchronous query backend for views.
2. **One WhatsApp connection.** Reconnect via whatsmeow `EnableAutoReconnect` + app status events. No parallel clients.
3. **Paginate history.** Default window ≈ 50 messages; load older on demand.
4. **Don't auto-download huge media.** Metadata first; download on demand (small images may auto-fetch later).
5. **Throttle/debounce expensive work**, never keystrokes for the input field.
6. **Logs go to files**, never stdout/stderr during TUI.

## Engine / UI boundary

```text
WhatsApp events
  → engine handler (goroutine)
  → persist to whatstui.db (worker)
  → EventBus non-blocking send
  → Bubble Tea Msg
  → patch UI state
  → render
```

If the EventBus is full, drop or coalesce low-priority events (e.g. sync progress), never block the engine on the UI.

## Future benchmarks (`benchmarks/`)

1. Startup with 100 / 1_000 chats
2. Open chat with 100 / 10_000 messages
3. Search
4. Typing under sync load
5. Conversation switch
6. Receive / send text
7. Send / download file

Phase 6 owns the harness. Phase 3 owns the optimizations those numbers will demand.
