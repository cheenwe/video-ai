import { ApiError, getToken, request } from './client'

export interface LicenseMeta {
  edition: string
  valid: boolean
  customer_name?: string
  license_id?: string
  expires_at?: string
  features: string[]
  max_users?: number
  user_count?: number
  reason?: string
  last_sync_at?: string
}

export interface WatermarkInfo {
  enabled: boolean
  text: string
  opacity: number
  font_size: number
  angle: number
  density?: number
  scope: string[]
}

export function normalizeWatermarkInfo(
  input?: Partial<WatermarkInfo> | null,
): WatermarkInfo | null {
  if (!input) return null
  const densityRaw = input.density
  const density = Number(densityRaw)
  return {
    enabled: !!input.enabled,
    text: input.text || '{username} · {datetime}',
    opacity: input.opacity ?? 0.15,
    font_size: input.font_size ?? 14,
    angle: input.angle ?? -25,
    density:
      densityRaw != null && Number.isFinite(density) && density > 0
        ? Math.min(2, Math.max(0.1, density))
        : 0.7,
    scope: input.scope?.length ? [...input.scope] : ['app', 'preview', 'share', 'download'],
  }
}

export interface SiteInfo {
  version: string
  edition: string
  max_users?: number
  user_count?: number
  license: LicenseMeta
  watermark?: WatermarkInfo
}

export function getSiteInfo() {
  return request<SiteInfo>('/api/site/info')
}

export function getLicenseStatus() {
  return request<LicenseMeta>('/api/admin/license/status')
}

export interface MachineFingerprint {
  machine_id: string
  short_machine_id: string
  version: string
  hostname: string
  platform: string
  sources: string[]
  stable: boolean
  overridden: boolean
}

export function getMachineFingerprint() {
  return request<MachineFingerprint>('/api/admin/license/fingerprint')
}

export function importLicenseFile(file: File) {
  const fd = new FormData()
  fd.append('file', file)
  return request<LicenseMeta>('/api/admin/license/import', { method: 'POST', body: fd })
}

export function activateLicense(licenseServerUrl: string, licenseId: string) {
  return request<LicenseMeta>('/api/admin/license/activate', {
    method: 'POST',
    body: JSON.stringify({ license_server_url: licenseServerUrl, license_id: licenseId }),
  })
}

export async function syncLicenseInfo() {
  const token = getToken()
  const res = await fetch('/api/admin/license/sync', {
    method: 'POST',
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  })
  const json = await res.json().catch(() => ({}))
  if (!res.ok || json.success !== 1) {
    throw new ApiError(json.msg || '同步失败', json.code || res.status)
  }
  return {
    meta: json.data as LicenseMeta,
    message: (json.msg as string) || '授权信息已同步',
  }
}

/** @deprecated use syncLicenseInfo */
export function syncLicenseCRL() {
  return syncLicenseInfo().then((r) => r.meta)
}

export function removeLicense() {
  return request<LicenseMeta>('/api/admin/license/remove', { method: 'POST' })
}

export interface TrialStatus {
  id?: number
  status: 'none' | 'pending' | 'approved' | 'rejected'
  edition?: string
  license_id?: string
  expires_at?: string
  message?: string
}

export function applyTrialLicense(
  licenseServerUrl: string,
  body: { team_name?: string; contact?: string; edition: string },
) {
  return request<TrialStatus>('/api/admin/license/trial/apply', {
    method: 'POST',
    body: JSON.stringify({ license_server_url: licenseServerUrl, ...body }),
  })
}

export function getTrialStatus(licenseServerUrl: string) {
  const q = encodeURIComponent(licenseServerUrl)
  return request<TrialStatus>(`/api/admin/license/trial/status?license_server_url=${q}`)
}
