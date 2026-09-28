# Local bridge (Codex + Cursor)

Single-process local bridge for read-only quota acquisition and outbound HTTPS uploads. One device credential serves both connectors; revoking it disconnects Codex and Cursor uploads together.

## Connectors

| Connector | Source | Notes |
|-----------|--------|-------|
| **Codex** | Local app-server | Existing read-only Codex quota path |
| **Cursor** | Windows: `%APPDATA%\Cursor\User\globalStorage\state.vscdb` + `api2.cursor.sh` | Token stays on device; only sanitized snapshots upload |

Non-Windows hosts can run the bridge for Codex; Cursor local auth is unavailable there (`cursor local auth unavailable on this platform`).

## Security

- Cookie, provider access token, prompts, responses, terminal output and full logs must not enter ordinary sync payloads.
- Cursor bearer tokens are used only to call `api2.cursor.sh` from the bridge process; they are never uploaded to the hub server.
- The quota-band transport is LAN-only and is not exposed on the Internet. Bridge uses HTTPS outbound transport and an independently revocable device credential.

## Operations

```bash
# Probe Cursor collector (Windows, logged into Cursor)
aihub-bridge --probe cursor

# One-shot upload for all enabled connectors
aihub-bridge --once

# Periodic collection (default interval in config)
aihub-bridge
```

Enable Cursor in bridge config: `cursor_enabled: true`. See [connection guide](../docs/CODEX_CONNECTION_ZH.md) for pairing and device credentials.
