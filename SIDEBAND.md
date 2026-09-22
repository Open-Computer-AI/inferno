# Inferno Live Sideband Runbook

Status: verified operational snapshot
Recorded: 2026-09-19
Repository: `/Users/saksham/OpenComputerV2/inferno-local`

## What this documents

Inferno creates the Live call through its normal HTTP gateway and then attaches a
server-side WebSocket sideband to the same upstream call. The browser/WebRTC
connection and the server-side sideband are separate connections, but they share
the upstream `call_id` and identity context.

## Reliability changes in the tracked source

The current working-tree changes in `backend/internal/service/openai_live.go`
and `backend/internal/service/openai_live_types.go` do the following:

1. Capture the upstream `session-id` and `thread-id` generated during Live call
   creation.
2. Store those values with the local Live call record.
3. Reuse the exact same values when opening the later sideband connection,
   instead of generating a second identity pair.
4. Continue carrying the existing attestation ciphertext with the call so the
   sideband can reuse the creation-session proof.
5. Record upstream status, response headers, and response body details when the
   sideband handshake fails, making 1011/Cloudflare/upstream failures diagnosable.
6. Add focused route and call-mapping tests.

The important invariant is:

```text
create Live call -> call_id + session-id + thread-id
sideband attach  -> same call_id + same session-id + same thread-id
```

## Verified runtime snapshot

The currently running local service is managed by launchd as `inferno-local` and
listens on `127.0.0.1:18080`.

The observed successful smoke run produced the service log event:

```text
OpenAI Live sideband connected
```

The successful candidate binary was preserved as:

- `sub2api-darwin-arm64.direct-sideband-observed`
- `sub2api-darwin-arm64.direct-sideband-candidate`

The currently active `sub2api-darwin-arm64` is the observed candidate copy.

The observed candidate uses the direct Live sideband target:

```text
wss://api.openai.com/v1/live/{call_id}
```

The direct-sideband target is now reconciled into tracked source as
`wss://api.openai.com/v1/live`; the focused lifecycle test asserts the same
target. Do not rebuild over the active binary without rerunning the verification
checklist below.

## Rollback artifacts

The pre-experiment binary is preserved as:

```text
sub2api-darwin-arm64.baseline-before-direct-sideband-test
```

Before any replacement, preserve the active binary. A manual rollback should:

1. Stop or unload the `inferno-local` launchd service.
2. Copy the current binary to a dated backup.
3. Restore `sub2api-darwin-arm64.baseline-before-direct-sideband-test` as the
   executable.
4. Restart `inferno-local`.
5. Verify `/health` and run the Live smoke test before using the service again.

Never delete the candidate or baseline artifacts.

## Frontend state and safe restoration

`127.0.0.1:18080` is currently the backend/API. Its `/health` endpoint returns
200, while `/` returns 404 because the active binary was built without the
embedded frontend.

The management frontend source still exists in `inferno-frontend`. The safest
restoration path is:

1. Build `inferno-frontend` separately into its `dist` output.
2. Serve it on a separate local port, pointing its API base at
   `http://127.0.0.1:18080`.
3. Keep the active sideband binary and launchd service untouched.
4. Verify account management, authorization, usage, and Live test flows.
5. Only after that, build an embedded frontend binary with the repository's
   `build-embed` path, carrying forward the verified sideband transport first.

The existing `4173` page is the separate Inferno Live test page, not the full
account-management frontend. The management frontend is now running separately
on `http://127.0.0.1:4174/` with:

```text
VITE_DEV_PROXY_TARGET=http://127.0.0.1:18080
VITE_DEV_PORT=4174
```

Verified after launch:

- `GET http://127.0.0.1:4174/` returns HTTP 200.
- The page title is `Inferno - AI API Gateway`.
- `GET http://127.0.0.1:4174/api/v1/settings/public` proxies successfully and
  returns HTTP 200.
- `GET http://127.0.0.1:18080/health` remains HTTP 200.
- The existing `4173` Live-test page remains HTTP 200.

Reusing `4173` for the management UI should wait until that test page has been
moved or its ownership is made explicit.

## Verification checklist

For every future binary or sideband change:

- `GET http://127.0.0.1:18080/health` returns HTTP 200.
- A real authorized Live session is created successfully.
- The sideband attaches to the same `call_id`.
- Logs contain `OpenAI Live sideband connected`.
- The session remains open while browser audio/transcripts flow.
- A controlled failure records upstream status and handshake diagnostics.
- The frontend management smoke test passes if the binary was rebuilt.

This file intentionally contains no credentials, API keys, OAuth tokens, or
account secrets.
