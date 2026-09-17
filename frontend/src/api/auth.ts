import { publicRequest, request } from './client'

export interface UserInfo {
  user_id: number
  username: string
  is_admin: boolean
  is_superadmin?: boolean
  role?: string
  is_system?: boolean
  real_name: string | null
  department_id?: number | null
  department_name?: string
  timezone: string
  avatar_url?: string | null
  totp_enabled?: boolean
  auth_source?: string
  must_change_password?: boolean
  must_setup_totp?: boolean
  permissions?: string[]
}

export interface LoginResult {
  token: string
  user: UserInfo
}

export interface LoginPendingTOTP {
  need_totp: true
  temp_token: string
}

export interface OAuthProviderPublic {
  id: string
  name: string
  type: string
  icon_url?: string
}

export interface AuthConfig {
  allow_register: boolean
  captcha_enabled: boolean
  oauth_enabled: boolean
  oauth_providers: OAuthProviderPublic[]
  password_rules: string[]
  ldap_enabled?: boolean
}

export interface CaptchaChallenge {
  captcha_id: string
  image_base64: string
  obfuscated?: boolean
}

export function fetchAuthConfig(username?: string) {
  const q = username?.trim() ? `?username=${encodeURIComponent(username.trim())}` : ''
  return publicRequest<AuthConfig>(`/api/auth/config${q}`)
}

export function fetchCaptcha() {
  return publicRequest<CaptchaChallenge>('/api/auth/captcha')
}

export function login(data: {
  username: string
  password: string
  captcha_id?: string
  captcha_answer?: string
}) {
  return publicRequest<LoginResult | LoginPendingTOTP>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function verifyTOTP(temp_token: string, code: string) {
  return publicRequest<LoginResult>('/api/auth/totp/verify', {
    method: 'POST',
    body: JSON.stringify({ temp_token, code }),
  })
}

export function register(data: { username: string; email: string; password: string }) {
  return publicRequest<{ username: string }>('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function forgotPassword(username: string) {
  return publicRequest<{ message: string; reset_token?: string; expires_at?: string }>(
    '/api/auth/forgot-password',
    { method: 'POST', body: JSON.stringify({ username }) },
  )
}

export function resetPassword(token: string, new_password: string) {
  return publicRequest<null>('/api/auth/reset-password', {
    method: 'POST',
    body: JSON.stringify({ token, new_password }),
  })
}

export function fetchMe() {
  return request<UserInfo>('/api/auth/me')
}

export function logout() {
  return request<null>('/api/auth/logout', { method: 'POST' })
}

export function updateProfile(data: { real_name?: string; timezone?: string }) {
  return request<UserInfo>('/api/auth/profile', {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export function changePassword(old_password: string, new_password: string) {
  return request<null>('/api/auth/change-password', {
    method: 'POST',
    body: JSON.stringify({ old_password, new_password }),
  })
}

export function fetchTimezones() {
  return request<{ items: string[] }>('/api/auth/timezones')
}

export function oauthStartUrl(providerId: string) {
  return `/api/auth/oauth/${encodeURIComponent(providerId)}`
}

export interface TotpStatus {
  enabled: boolean
  enforced: boolean
}

export function totpStatus() {
  return request<TotpStatus>('/api/auth/totp/status')
}

export function totpSetup() {
  return request<{ secret: string; otp_url: string; qr_url: string }>('/api/auth/totp/setup', {
    method: 'POST',
  })
}

export function totpConfirm(code: string) {
  return request<null>('/api/auth/totp/enable', {
    method: 'POST',
    body: JSON.stringify({ code }),
  })
}

export function totpDisable(code: string) {
  return request<null>('/api/auth/totp/disable', {
    method: 'POST',
    body: JSON.stringify({ code }),
  })
}
