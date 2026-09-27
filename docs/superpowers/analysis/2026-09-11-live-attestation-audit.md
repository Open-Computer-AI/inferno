# Live Attestation Audit Specification

## 1. HEADER

- reqid: f6da8021
- repo: /Users/architsakri/OpenComputerV2/inferno
- branch: inferno-redesign
- BASE_SHA: 4da86dec184ea3bf2ef62ac951a1016b817a6dcf
- date: 2026-09-11
- COMPONENT: general
- purpose: fact-only audit; no implementation change is proposed

## 2. PROBLEM

The repository exposes OpenAI Live and Codex realtime-call routes and accepts OpenAI OAuth accounts for the Live capability. The upstream Live call contract additionally requires an `x-oai-attestation` header. This audit records how that header is produced, which credentials and local runtime assets are independently required, and the smallest supported way to make a Live session available. It does not test or bypass the upstream service and does not use secrets.

## 3. GOALS / NON-GOALS

Goals:
- Identify the Live request routes and the service call path.
- Identify the attestation provider, its platform gates, and its required ChatGPT application assets.
- Determine whether Codex/OpenAI OAuth alone is sufficient to create a Live voice session.
- State the smallest supported workaround when the attestation runtime is unavailable.

Non-goals:
- No code, route, authentication, or attestation changes.
- No attempt to synthesize, replay, bypass, or weaken DeviceCheck attestation.
- No live upstream request, production credential use, or installation/deployment action.
- No claim about undocumented upstream behavior beyond the repository evidence listed here.

## 4. RISK

RISK: HIGH for any implementation change, because this is an authentication/attestation boundary and the upstream contract is external. This audit itself is LOW risk because it is read-only and changes no tracked source.

- Blast radius: HIGH if changed; Live routes, account capability selection, and sideband authentication are shared service paths.
- Reversibility: LOW for this audit; any future code change is revertible, but an attestation bypass would be an unacceptable security regression.
- Novelty: HIGH at the repository boundary; macOS DeviceCheck and the official ChatGPT application runtime are external platform dependencies.

## 5. ALTERNATIVES

1. Use the existing provider unchanged and install/retain the required official ChatGPT app runtime on the Sub2API host; smallest, repository-consistent, and preserves the attestation boundary.
2. Replace the provider with a token supplied by Codex OAuth; rejected by repository evidence because the provider independently calls Apple DeviceCheck and packages its result into the required header.
3. Add a bypass or synthetic header; rejected because it would defeat the upstream device-attestation check and is explicitly outside this audit.

## 6. APPROACH

Observed source path, in order:

1. `backend/internal/server/routes/gateway.go:376-380` registers `POST /backend-api/codex/realtime/calls` and `GET /backend-api/codex/:call_id`.
2. The general gateway route registers `/live`; `backend/internal/handler/openai_live.go:22-123` authenticates the API key, requires an OpenAI or Composite group with Live enabled, validates the SDP/session payload, and calls `CreateLiveCall`.
3. `backend/internal/service/openai_gateway_service.go:558-559` constructs the service with `liveattestation.NewProvider()` and an AES-GCM sideband-attestation cipher derived from the configured JWT secret.
4. `backend/internal/service/openai_live.go:124-143` calls `prepareLiveAttestation` before account selection or the upstream call. `prepareLiveAttestation` invokes `Provider.Generate`; failure becomes `LiveAttestationUnavailableError` and prevents Live creation.
5. `backend/internal/service/openai_live.go:261-305` obtains the selected account's access token, sends the request to `https://chatgpt.com/backend-api/codex/realtime/calls?intent=quicksilver&architecture=avas`, and sets `x-oai-attestation` to the generated value.
6. `backend/internal/service/account.go:1815-1821` allows `OpenAIEndpointCapabilityLive` only for OpenAI OAuth accounts, excluding personal access tokens and agent-identity accounts. This is an account capability gate, not the attestation source.
7. `backend/internal/service/openai_live.go:416-437` obtains the access token again for the sideband, decrypts the stored attestation, and sends both authentication and `x-oai-attestation` to `wss://chatgpt.com/backend-api/codex/<call_id>`.
8. `backend/internal/platform/liveattestation/attestation_darwin.go:50-59` creates the Darwin provider and searches `/Applications/ChatGPT.app` and `~/Applications/ChatGPT.app`.
9. `attestation_darwin.go:68-91` requires Apple Silicon (`runtime.GOARCH == arm64`), the app bundle, `Contents/Resources/cua_node/bin/node`, `Contents/Resources/native/devicecheck.node`, and a `com.openai.*` bundle identifier read using `/usr/bin/plutil`.
10. `attestation_darwin.go:94-138` reads local macOS locale/language/timezone/screen signals, invokes the ChatGPT-bundled Node runtime with the native DeviceCheck module, and returns a JSON attestation containing a generated DeviceCheck token and fingerprint. On non-Darwin builds, `attestation_unsupported.go` always returns `ErrUnsupportedPlatform`.

Facts answering the audit questions:

- Live attestation is a short-lived, locally generated JSON header (`x-oai-attestation`) containing a versioned `v1.` token. The token is built from an Apple DeviceCheck token, the official app bundle identifier, and bounded macOS device signals. The repository calls it ChatGPT DeviceCheck attestation.
- Codex/OpenAI OAuth alone cannot create a Live session through this implementation. OAuth supplies the upstream bearer authentication and account identity, but `prepareLiveAttestation` must independently succeed before the request is sent. Live capability also explicitly excludes API-key, personal-access-token, and agent-identity account modes.
- ChatGPT app files are required on the Sub2API server when running on macOS: the official bundle, its bundled Node executable, its native `devicecheck.node` module, and a valid `com.openai.*` bundle identifier. The repository does not load a token from a user's ChatGPT app account; it executes the app-bundled runtime and native module locally.
- The smallest supported workaround is operational, not a credential workaround: run Sub2API on Apple Silicon macOS and install the official ChatGPT app in `/Applications/ChatGPT.app` (or `~/Applications/ChatGPT.app`) so the required resources exist. Keep a supported OpenAI OAuth account for upstream authorization, configure the JWT secret so sideband attestation encryption is available, enable Live for the group, and use an account whose capability is OpenAI OAuth. No repository code change is required for this path.
- The repository exposes `GET .../live-capability` via the admin group handler; it calls `NewProvider().Check` and reports `supported` plus a reason. This is the supported preflight rather than an attempt to infer capability from OAuth state.

ASSUMPTIONS:
- “Codex OAuth” means the repository's OpenAI `AccountTypeOAuth` account path, not an undocumented token from a separate client.
- “ChatGPT app files” means the official app bundle and the exact bundled runtime/module paths inspected by `resolveRuntime`; no claim is made that arbitrary copied files are supported.
- This audit answers repository-supported behavior only; upstream policy or future app/runtime changes require a fresh source audit.

## 7. TASKS

1. Trace route and handler wiring; completed from `gateway.go` and `openai_live.go`.
2. Trace credential/capability gates and attestation lifecycle; completed from `account.go`, `openai_live.go`, `openai_live_attestation.go`, and `openai_gateway_service.go`.
3. Inspect Darwin and non-Darwin provider requirements; completed from `attestation.go`, `attestation_darwin.go`, and `attestation_unsupported.go`.
4. Record the supported operational workaround and explicit non-workarounds; completed in this document.

## 8. ACCEPTANCE CRITERIA

This is a fact-only audit, so criteria are evidence checks rather than a proposed code delta. No RED implementation criterion is applicable and no source test is added.

| id | command | baseline expected/observed | proves |
|---|---|---|---|
| AC1 | `go test ./internal/platform/liveattestation` | GREEN / not run in planning | Existing platform-provider tests preserve the explicit unsupported-platform behavior; Darwin-specific generation is not exercised without the official app and DeviceCheck runtime. |
| AC2 | `go test ./internal/service` | GREEN / not run in planning | Existing service tests cover Live attestation encryption, explicit provider errors, upstream header construction, and OAuth-only Live capability. |
| AC3 | `go test ./internal/handler` | GREEN / not run in planning | Existing handler tests cover Live request parsing, route-side policy checks, and explicit attestation-unavailable error mapping. |

## 9. TEST STRATEGY

- No test files are changed because this request is an audit, not an implementation.
- Existing evidence files are `backend/internal/platform/liveattestation/attestation_unsupported_test.go`, `backend/internal/service/openai_live_test.go`, `backend/internal/service/openai_live_lifecycle_test.go`, and `backend/internal/handler/openai_live_test.go`.
- The Darwin provider's actual DeviceCheck generation requires the official app and Apple runtime assets and is not invoked by this audit.
- No candidate tests, network calls, credentials, or production state were used.
- Existing tests are immutable — weakening or deleting one is an automatic REJECT

## 10. ROLLOUT / ROLLBACK

No rollout or rollback applies because this audit makes no code or configuration change. If a future implementation is proposed, deployment is only through the fixed root-owned `/opt/opencomputer/deploy-control/eng_deploy` launcher behind the human gate; recovery is `git revert <commit>` plus a full pipeline rerun and a fresh gate. A future change must preserve the fail-closed behavior when the provider, official app, Apple Silicon, DeviceCheck module, bundle identifier, or JWT secret is unavailable.

## 11. OUT OF SCOPE

- Fabricating or replaying `x-oai-attestation`.
- Reusing a Codex OAuth token as a substitute for DeviceCheck.
- Removing the official-app or Apple-Silicon requirement.
- Live upstream probing, production deployment, secret inspection, or installing software.
- Claiming that ChatGPT account files, browser cookies, or arbitrary copied native modules are supported.
