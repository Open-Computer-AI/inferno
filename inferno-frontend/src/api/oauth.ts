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
