# Pine for VS Code

Open your [Pine](https://github.com/underworld14/pine) board — a git-native,
local-first kanban & issue tracker — **directly inside VS Code**. No terminal, no
`pine serve` by hand: the extension starts Pine for you and shows the board in an
editor tab, reusing the exact same web UI you get in the browser.

## Features

- **Pine: Open Board** — opens the kanban board in a VS Code tab. Drag cards, edit
  tickets, upload attachments — everything works, with live sync.
- **Pine: Create Bug** / **Pine: Create Feature** — create a ticket from the
  command palette; an open board updates live.
- **Status bar** entry and automatic detection of a `.pine/` workspace.

## How it works

The extension finds the `pine` binary (on your `PATH`, or downloads the matching
release), starts `pine serve` on a local port, and embeds that localhost URL in a
webview. Because the board runs from `http://127.0.0.1:<port>` inside the frame,
the whole Svelte UI — API calls, live SSE updates, attachments — works unchanged.
When you close the board tab, the server the extension started is stopped; a Pine
server you started yourself is left running and reused.

## Requirements

- A workspace initialized with `pine init` (contains a `.pine/` directory).
- The `pine` binary. If it is not on your `PATH`, the extension offers to download
  the correct build from GitHub Releases (checksum-verified) on first use.

## Settings

| Setting | Default | Description |
| --- | --- | --- |
| `pine.path` | `""` | Absolute path to the `pine` binary. Overrides PATH lookup and auto-download. |
| `pine.autoDownload` | `true` | Download the matching release binary if `pine` is not found on PATH. |
| `pine.server.port` | `null` | Preferred port for `pine serve`. Empty auto-selects a free port. |

## Development

```bash
npm install
npm run watch      # rebuild dist/extension.js on change
# press F5 in VS Code to launch the Extension Development Host
npm test           # unit + e2e (drives a real `pine` binary; set PINE_BIN)
npm run test:integration   # launches a headless VS Code
npm run package    # produce a .vsix
```

Set `PINE_BIN` to a built `pine` (e.g. `make build` at the repo root produces
`./pine`) so the e2e/integration tests can drive it.

## Known limitations

- Targets **local desktop VS Code**. Remote/Codespaces needs a Pine-side host
  allow-list change (planned).
- The board keeps its own theme toggle; it does not yet follow the VS Code theme.

## License

MIT © underworld14
