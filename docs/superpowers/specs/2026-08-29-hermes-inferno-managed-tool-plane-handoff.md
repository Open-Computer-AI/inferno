# Hermes ↔ Inferno managed tool plane — handoff and planning brief

**Date:** 2026-08-29
**Status:** Planning / handoff. This document does not implement or change the
runtime.
**Canonical client:** `oc-hermes` (the Hermes fork used by OpenComputer).
**Canonical backplane:** Inferno.
**Explicitly out of scope:** the legacy `OpenComputerV2` agent surface and
`router.tryopencomputer.com`.

## Purpose

This is the durable handoff for making Inferno provide a Nous-Portal-like
managed tool plane in addition to its existing model/inference plane.

The intended result is not to move every Hermes capability into Inferno. Hermes
remains the front door and orchestrator. Inferno centralizes only the
provider-backed capabilities that need shared credentials, account selection,
entitlements, quotas, billing, failover, and operational visibility.

A fresh coding agent should read this document together with the source files in
the final section before proposing implementation details. The source code is
the authority; names and endpoint shapes in this document marked as “candidate”
are not frozen contracts.

## Executive decision

The system should have two clearly separated tool classes:

1. **Hermes-local tools:** tools that act on the agent host or on OpenComputer's
   operational systems. Examples are terminal/process execution, files and
   patches, Linear, Slack/Telegram actions, memory, delegation, kanban, and
   local computer use.
2. **Inferno-managed tools:** provider-backed capabilities that should be
   shared and metered centrally. Examples are web search/extraction, selected
   image/video/audio providers, and an explicitly scoped remote browser service
   if that proves useful.

The model call and the tool call are separate legs. A model list must not be
used as a tool catalog, and adding a tool to `/v1/models` is not the design.

## Verified current state

### Hermes / `oc-hermes`

- The Hermes tool registry exposes `web_search` and `web_extract` through the
  web toolset. The shared core tool list also makes them available to the
  normal CLI and messaging-platform tool posture when their availability check
  passes.
- `web_search_tool()` resolves a registered search provider and executes that
  provider in the Hermes process. It currently supports multiple provider
  plugins rather than calling Inferno's `/v1/web_search` automatically.
- Hermes also contains a generic managed-tool gateway path. That path can use a
  Nous Portal OAuth access token for entitled vendor gateways such as Firecrawl,
  FAL, OpenAI audio, browser-use, and Modal. This is separate from ordinary
  model inference.
- On the dedicated Mac, a read-only runtime check resolved the active Hermes
  search backend to `keenable`. A direct invocation of Hermes's
  `web_search_tool` with a harmless query returned `success: true` and three
  results. This proves the Hermes tool handler works; it does **not** prove a
  full natural-language agent turn chose the tool, and it did not exercise
  Inferno's Grok-native search route.

### Inferno

- Inferno already owns the model/inference plane: provider accounts, OAuth/API
  authentication, groups, composite model routing, quotas, billing, scheduling,
  failover, and model gateway endpoints.
- Inferno already has a `POST /v1/web_search` route. Its current implementation
  is provider-specific: it requires a Grok group, selects a Grok account, calls
  Grok's native `web_search` capability, normalizes sources, and records usage.
- Inferno has a plugin framework and administrative configuration surfaces, but
  that is not yet the same thing as a general user-facing tool catalog and
  entitlement system.
- The current Inferno OAuth/account design deliberately omits `tool_access`
  because Inferno does not yet model per-tool entitlements. Do not fabricate a
  permissive `tool_access` object before the entitlement model exists.

### Current search paths

There are currently two distinct search paths:

```text
Hermes web_search tool → Hermes-selected provider (currently Keenable)

Direct Inferno API call → /v1/web_search → Grok group → Grok native search
```

Neither path should be silently confused with the other. If centralized
Inferno-managed search is enabled later, Hermes must use an explicit Inferno
adapter and the logs must make that path observable.

## Target request flow

The intended end-to-end flow is:

```text
User in Slack / Telegram / Desktop
        ↓
Hermes gateway and active profile
        ↓
Inferno model endpoint (/v1/messages, /v1/chat/completions, /v1/responses, ...)
        ↓
Model emits a structured tool call, for example web_search(query)
        ↓
Hermes tool executor
        ├─ Hermes-local tool → executes locally
        └─ Inferno-managed tool → authenticated Inferno tool endpoint
                                      ↓
                              entitlement and quota check
                                      ↓
                              provider/account selection
                                      ↓
                              provider execution and normalization
                                      ↓
                              usage, billing, audit, and result
        ↓
Hermes adds the tool result to the conversation
        ↓
Hermes calls Inferno again for the final model response
        ↓
Response is sent back to the user
```

Inferno therefore becomes the source of truth for centrally managed tool
access, while Hermes remains responsible for deciding when the model should
invoke a tool and for continuing the conversation after the result returns.

## Responsibility boundary

| Area | Hermes / `oc-hermes` | Inferno |
| --- | --- | --- |
| Tool schemas | Presents schemas to the model and validates the local call shape | Publishes or validates the managed-tool contract |
| Orchestration | Decides when to call a tool, executes the tool loop, returns the final answer | Does not own the conversation loop |
| Local actions | Terminal, files, Linear, messaging, memory, delegation, kanban, computer use | Never receives the host's private execution authority by default |
| Provider-backed actions | Thin adapter and result reinsertion | Provider adapters, account selection, failover, concurrency, normalization |
| Identity | Uses the active user's OAuth session/token | Authorizes the user/group and enforces tool permissions |
| Usage | Records client-side diagnostics without being authoritative | Authoritative usage, quota, billing, provider/account attribution, audit |
| Operations | Surfaces structured errors to the user | Health, metrics, rate limits, provider failures, admin controls |

`oc-platform` is not part of this tool-plane change. It remains the compute/VM
plane and should only be involved when a future tool explicitly needs VM
lifecycle or sandbox execution.

## Proposed implementation scope

### Workstream A — contract discovery and design lock

Before writing the first implementation, the next agent must read the actual
Hermes consumers and parsers and document:

- how tool schemas are assembled for Slack, Telegram, Desktop, cron, and
  delegated workers;
- how Hermes currently resolves the active OAuth token and profile;
- how tool errors and multimodal results are inserted back into the model
  conversation;
- which current provider plugins already have a managed-gateway seam;
- which Inferno middleware performs authentication, composite targeting,
  scheduling, usage recording, and billing.

Do not freeze an endpoint or JSON envelope from this document alone. The first
design artifact should define a versioned request/result contract after those
consumers are verified.

Candidate conceptual shapes are:

```text
Tool definition:  id, version, description, input_schema, output_contract,
                  required_capability, supported_modes

Invocation:       request_id, tool_id, version, input, user/group context,
                  idempotency key, timeout/deadline

Result:           request_id, ok, output, sources/attachments, usage metadata,
                  provider metadata, structured error when not ok
```

The exact wire names, authentication headers, streaming behavior, and maximum
payload sizes must come from the real Hermes caller and Inferno middleware
contracts.

### Workstream B — Inferno tool control plane

Add the server-side primitives required for managed tools:

- versioned tool catalog and capability metadata;
- user/group-level tool entitlement and enablement model;
- OAuth authorization for tool invocation, with an explicit tool permission
  rather than assuming that model inference permission means every tool is
  allowed;
- provider adapter interface and account selection;
- concurrency, timeout, retry, and provider failover behavior;
- authoritative usage records and billing dimensions for tool invocations;
- redacted audit events and operational metrics;
- admin APIs/UI for tool availability, group assignment, pricing, usage, and
  provider health.

The control plane must fail closed before contacting an upstream provider when
the caller is unauthorised or the tool is disabled. It must not return a
permissive capability object merely because an older client expects a field.

### Workstream C — first vertical slice: web search and extraction

Implement one complete managed-tool path before expanding the surface:

1. Define the managed `web_search` request/result contract.
2. Decide whether the first Inferno provider is the existing Grok-native
   implementation, a Keenable adapter, Firecrawl, or another provider. This is
   a product and billing decision, not an accidental fallback.
3. Generalize the current Grok-specific handler behind a provider interface if
   that is the selected starting point.
4. Add entitlement, quota, usage, source normalization, timeout, and audit
   tests.
5. Add `web_extract` only after its URL-safety, provider, size-limit, and
   billing behavior are explicitly defined.
6. Add an explicit Hermes backend/adapter that calls local Inferno for these
   tools.
7. Prove the full loop through a real Hermes profile, not only a direct handler
   unit test.

When Inferno-managed mode is selected, there must be no hidden fallback to
Keenable, Nous's old hosted router, or `router.tryopencomputer.com`. A fallback
must be an explicit profile/configuration choice and must be visible in logs.

### Workstream D — extend provider-backed capabilities

After the web vertical slice is stable, evaluate these in order:

- image generation/editing;
- video generation and asynchronous task polling;
- audio transcription and text-to-speech;
- managed browser-use, only where remote browser execution is actually desired;
- Modal or other remote compute-backed tools, only with a clear tenant and
  sandbox boundary.

The existing Hermes-local implementations should remain available for tools
whose value depends on the dedicated Mac's filesystem, GUI, credentials, or
agent process. Centralizing a capability is not automatically an instruction to
remove its local implementation.

## Security and reliability invariants

The next agent must preserve these invariants:

- Provider OAuth credentials and upstream secrets remain inside Inferno or the
  explicitly designated Hermes provider store; they are never returned in tool
  results or logs.
- Hermes sends a user-scoped authorization token to Inferno. Inferno derives
  user, group, entitlement, quota, and billing context server-side.
- Web extraction and browser tools enforce SSRF/private-network protections,
  URL normalization, size limits, timeouts, and content-safety handling.
- Every invocation has a request ID and an idempotency strategy appropriate to
  the tool. Retries must not duplicate billable mutations.
- Provider failover is bounded and observable. Do not retry a non-retryable
  policy, authorization, or malformed-input failure against another provider.
- A tool failure returns a structured tool error to Hermes. Hermes must not
  present an invented successful result.
- Tool results are size-bounded and sanitized before being reintroduced into a
  model context. Untrusted web content must remain data, not instructions.
- Model composite routing and tool routing are separate decisions. A model's
  provider alias must not be used as an implicit authorization for unrelated
  tools.
- The old production router is not an internal backup path.

## Phased delivery plan

### Phase 0 — evidence and contract

- Verify the current Hermes profile/toolset assembly and active Inferno base URL
  on the dedicated Mac.
- Map all tool callers, result parsers, middleware, and auth refresh paths.
- Write the versioned contract and threat model.
- Decide the first provider and billing unit.

### Phase 1 — Inferno foundations

- Add the smallest tool catalog and entitlement model needed for one tool.
- Add authenticated invocation, usage/audit records, provider adapter seams, and
  admin visibility.
- Add unit and integration tests before wiring Hermes to the new path.

### Phase 2 — web-search vertical slice

- Implement managed `web_search` end to end.
- Add `web_extract` if its provider and URL-safety contract are ready.
- Keep current Hermes-native search available only as an explicit mode while
  comparing results and failure behavior.

### Phase 3 — Hermes adapter and full proof

- Add the explicit Inferno-managed backend in `oc-hermes`.
- Run a real model → tool call → Inferno → tool result → final model response
  test through the active profile.
- Verify Slack, Telegram, Desktop, cron, and delegated-profile tool scopes do
  not accidentally broaden.

### Phase 4 — expansion and cleanup

- Add the next provider-backed tool categories one at a time.
- Add customer/group policies for LMI and internal users.
- Remove duplicate paths only after parity, migration evidence, and rollback
  behavior are documented.

## What is explicitly not part of this handoff

- Moving terminal, file editing, Linear, Slack/Telegram, memory, delegation,
  kanban, or local computer use into Inferno.
- Adding tools to the model list or pretending that `/v1/models` is a tool
  catalog.
- Reintroducing `router.tryopencomputer.com` as a backup or internal route.
- Implementing automatic cross-provider “AUTO” model routing.
- Creating a broad `tool_access` field in Inferno without a real entitlement
  model behind it.
- Rebuilding the Hermes conversation loop inside Inferno.
- Changing the current runtime or production configuration as part of this
  planning document.

## Acceptance criteria for the first shipped slice

The first managed tool should not be called complete until all of the following
are demonstrated:

1. An entitled user can invoke the tool through the active Hermes profile.
2. The model request still travels through local Inferno, and the tool request
   travels through the explicit Inferno managed-tool path.
3. An unentitled or disabled user receives a structured authorization error and
   no upstream provider call is made.
4. Inferno records the user, group, tool, provider, account, request ID, status,
   latency, and billable usage without exposing secrets.
5. Provider timeout, rate-limit, malformed-input, and upstream-auth failures
   have distinct, tested behavior.
6. The tool result is safely reinserted into Hermes and the final response is
   generated by the configured Inferno model route.
7. A live end-to-end test proves the path; a direct handler test alone is not
   accepted as proof.
8. Search for the legacy router returns no active internal call path.
9. Existing Hermes-local tools continue to work and no profile receives tools it
   was not explicitly granted.

## Open decisions for the next planning agent

1. Should the first centralized search provider be Grok-native, Keenable,
   Firecrawl, or a provider-neutral abstraction with one initial adapter?
2. Should Hermes keep its built-in tool schemas while Inferno only authorizes
   invocations, or should Hermes discover managed schemas from an Inferno tool
   catalog?
3. What exact OAuth permission and group entitlement should gate tool use?
4. Should internal unlimited and LMI use the same tool policy with different
   billing, or different allowed tool sets?
5. Is web search billed per invocation, per result, by provider usage, or by a
   combination of those units?
6. Which browser actions must remain local, and which—if any—should execute in a
   managed remote browser service?
7. Which tools need streaming or asynchronous job APIs rather than one bounded
   JSON response?
8. What is the explicit failure policy when Inferno-managed search is down: fail
   closed, or let a profile explicitly choose Hermes-native search?

## Handoff instructions

For the next coding/planning agent:

1. Treat this document as a planning brief, not as an implementation contract.
2. Read the Hermes consumer before changing an Inferno producer contract.
3. Verify the dedicated Mac and active profile; do not use the local personal
   machine as a substitute for the deployment source of truth.
4. Inspect current branches, worktrees, and uncommitted changes before editing.
5. Keep Hermes and Inferno changes in focused, reviewable commits.
6. Do not read, print, copy, or commit secrets. Use existing auth paths without
   exposing tokens.
7. Update this document's decision log or add a dated follow-up spec once the
   open decisions are resolved.

## Source-of-truth files inspected

Hermes:

- `~/.hermes/hermes-agent/toolsets.py`
- `~/.hermes/hermes-agent/tools/web_tools.py`
- `~/.hermes/hermes-agent/model_tools.py`
- `~/.hermes/hermes-agent/agent/tool_executor.py`
- `~/.hermes/hermes-agent/tools/managed_tool_gateway.py`
- `~/.hermes/hermes-agent/hermes_cli/nous_account.py`
- `~/.hermes/hermes-agent/hermes_cli/auth.py`

Inferno:

- `backend/internal/server/router.go`
- `backend/internal/server/routes/gateway.go`
- `backend/internal/handler/gateway_web_search.go`
- `docs/COMPOSITE_GROUPS.md`
- `docs/superpowers/specs/2026-08-19-billing-contract-adapter-design.md`
