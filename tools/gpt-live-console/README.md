# GPT Live local console

Disposable loopback-only developer console for manually exercising GPT Live WebRTC audio. It uses a project API key entered at runtime; the key is never stored, placed in a URL, rendered in status, or logged. GPT Live usage may be paid. No automated stage performs a live request.

## Run

From this directory, run `node server.mjs`, then open the printed `http://127.0.0.1:<port>/` URL in a browser and grant microphone access. Press Stop to send `session.close`; cleanup is bounded to 15 seconds and reports incomplete finalization if needed.

## Tests

From the repository root:

`pnpm --dir inferno-frontend exec vitest run --config ../tools/gpt-live-console/vitest.config.ts server.spec.ts`

`pnpm --dir inferno-frontend exec vitest run --config ../tools/gpt-live-console/vitest.config.ts app.spec.ts`

Tests use fakes only. They do not contact OpenAI, access a real microphone, or create a paid session. This tool is not production-safe and must remain localhost-only.
