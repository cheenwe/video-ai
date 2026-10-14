import { request } from './client'

export interface UserRow {
  id: number
  username: string
  email?: string | null
  role: string
  disabled: number
  web_login_enabled?: boolean
  real_name: string | null
  department_id?: number | null
  department_name?: string
  timezone: string
  avatar_url?: string | null
  created_at: string
  is_system?: boolean
  auth_source?: string
}

import type { PageResult } from './pagination'

export type { PageResult } from './pagination'

export function listUsers(params: { page?: number; page_size?: number; keyword?: string }) {
  const q = new URLSearchParams()
  if (params.page) q.set('page', String(params.page))
  if (params.page_size) q.set('page_size', String(params.page_size))
  if (params.keyword) q.set('keyword', params.keyword)
  return request<PageResult<UserRow>>(`/api/users?${q}`)
}

export function createUser(data: {
  username: string
  email: string
  password: string
  role?: string
  real_name?: string
  department_id?: number | null
  timezone?: string
  web_login_enabled?: boolean
}) {
  return request<UserRow>('/api/users', { method: 'POST', body: JSON.stringify(data) })
}

export function updateUser(
  id: number,
  data: Partial<{
    username: string
    email: string
    role: string
    real_name: string
    department_id: number | null
    new_password: string
    timezone: string
    web_login_enabled: boolean
  }>,
) {
  return request<UserRow>(`/api/users/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

export function setUserDisabled(id: number, disabled: boolean) {
  return request<UserRow>(`/api/users/${id}/disabled`, {
    method: 'POST',
    body: JSON.stringify({ disabled }),
  })
}

export function deleteUser(id: number) {
  return request<null>(`/api/users/${id}`, { method: 'DELETE' })
}

export function resetPassword(id: number) {
  return request<null>(`/api/users/${id}/reset-password`, { method: 'POST' })
}
