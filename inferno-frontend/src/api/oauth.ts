/**
 * OAuth device-flow approval API.
 *
 * ApproveDevice/DenyDevice (backend/internal/handler/oauth_handler.go) are
 * panel-only endpoints consumed exclusively by DeviceApprovalView -- unlike
 * every other /api/oauth endpoint, they are session-authenticated and wrapped
 * in internal/pkg/response's {code,message,data} envelope, which apiClient's
 * response interceptor already unwraps/normalizes the same way it does for
 * the rest of the panel API. They live under /api/oauth, not /api/v1
 * (apiClient's baseURL), so the request targets an absolute URL built by
 * buildGatewayUrl rather than a bare relative path.
 */
import { apiClient, buildGatewayUrl } from './client'

export interface DeviceDecisionResult {
  status: 'approved' | 'denied'
}

export async function approveDevice(userCode: string): Promise<DeviceDecisionResult> {
  const { data } = await apiClient.post<DeviceDecisionResult>(
    buildGatewayUrl('/api/oauth/device/approve'),
    { user_code: userCode }
  )
  return data
}

export async function denyDevice(userCode: string): Promise<DeviceDecisionResult> {
  const { data } = await apiClient.post<DeviceDecisionResult>(
    buildGatewayUrl('/api/oauth/device/deny'),
    { user_code: userCode }
  )
  return data
}

/**
 * What the human is being asked to approve, resolved from the user_code they
 * typed or that verification_uri_complete prefilled.
 *
 * RFC 8628 section 5.4 requires the authorization server to display client and
 * authorization information at the verification URI. Without it the approval
 * screen was a bare code box and two buttons -- indistinguishable from an
 * ordinary login prompt, which is exactly what makes a phished user_code
 * dangerous: the victim approves without ever seeing who is asking or what
 * they are handing over.
 *
 * Session-authenticated like approveDevice/denyDevice, so an unauthenticated
 * holder of a code cannot use it to confirm the code is live.
 */
export interface PendingDeviceAuthorization {
  client_name: string
  client_id: string
  scopes: string[]
  expires_at: string
}

export async function fetchPendingDeviceAuthorization(
  userCode: string
): Promise<PendingDeviceAuthorization> {
  const { data } = await apiClient.get<PendingDeviceAuthorization>(
    buildGatewayUrl('/api/oauth/device/pending'),
    { params: { user_code: userCode } }
  )
  return data
}

/**
 * GET/POST /oauth/authorize (backend/internal/handler/oauth_authorize_handler.go)
 * -- the RFC 6749 §4.1 authorization_code + PKCE browser leg.
 *
 * DELIBERATELY NOT on the {code,message,data} envelope, and NOT JSON at all:
 * that endpoint is a real OAuth authorization endpoint any client can
 * navigate a browser to directly, so its success/redirect-with-error
 * responses are a bare `text/plain` body carrying the exact URL to navigate
 * to next (the browser cannot follow a literal cross-origin 302 issued to a
 * `fetch()` call the way it could a real top-level navigation -- see that
 * handler's doc comment for the full reasoning). AuthorizeConsentView.vue's
 * only job on a "redirect" result is `window.location.assign(url)`.
 *
 * checkAuthorize is the GET: it reports either "here is where to go next"
 * (auto-approved, or the request was rejected with an RFC redirect-class
 * error) or "202, show the consent screen". decideAuthorize is the
 * consent screen's explicit Approve/Deny action (POST); both always resolve
 * to a target URL.
 */
export interface AuthorizeParams {
  response_type: string
  client_id: string
  redirect_uri: string
  scope: string
  state: string
  code_challenge: string
  code_challenge_method: string
}

export type AuthorizeCheckResult =
  | { kind: 'redirect'; url: string }
  | { kind: 'consent_required' }

export async function checkAuthorize(params: AuthorizeParams): Promise<AuthorizeCheckResult> {
  const response = await apiClient.get<string>(buildGatewayUrl('/oauth/authorize'), {
    params,
    responseType: 'text',
  })
  if (response.status === 202) {
    return { kind: 'consent_required' }
  }
  return { kind: 'redirect', url: response.data }
}

export async function decideAuthorize(
  params: AuthorizeParams,
  decision: 'approve' | 'deny'
): Promise<string> {
  const response = await apiClient.post<string>(
    buildGatewayUrl('/oauth/authorize'),
    null,
    { params: { ...params, decision }, responseType: 'text' }
  )
  return response.data
}

/**
 * Display-only "who is asking" lookup for the consent screen -- unlike
 * checkAuthorize/decideAuthorize above, this IS on the panel envelope (see
 * OAuthHandler.PendingAuthorization's doc comment).
 */
export interface PendingAuthorization {
  client_name: string
  client_id: string
}

export async function fetchPendingAuthorization(clientID: string): Promise<PendingAuthorization> {
  const { data } = await apiClient.get<PendingAuthorization>(
    buildGatewayUrl('/api/oauth/authorize/pending'),
    { params: { client_id: clientID } }
  )
  return data
}
