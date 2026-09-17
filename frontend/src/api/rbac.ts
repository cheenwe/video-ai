import { request } from './client'

export interface Role {
  id: number
  code: string
  name: string
  description?: string
  data_scope: string
  is_builtin: boolean
  sort_order: number
  enabled: boolean
  created_at?: string
  updated_at?: string
}

export interface PermissionItem {
  id: number
  code: string
  name: string
  module: string
  type: string
  sort_order: number
}

export interface PermissionModule {
  module: string
  items: PermissionItem[]
}

export function listRoles() {
  return request<{ items: Role[] }>('/api/roles')
}

export function createRole(body: { code: string; name: string; description?: string; data_scope?: string }) {
  return request<Role>('/api/roles', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function updateRole(id: number, body: { name?: string; description?: string; data_scope?: string; enabled?: number }) {
  return request<Role>(`/api/roles/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

export function deleteRole(id: number) {
  return request<null>(`/api/roles/${id}`, { method: 'DELETE' })
}

export function listPermissions() {
  return request<{ modules: PermissionModule[] }>('/api/permissions')
}

export function getRolePermissions(roleId: number) {
  return request<{ permissions: string[] }>(`/api/roles/${roleId}/permissions`)
}

export function assignRolePermissions(roleId: number, permissions: string[]) {
  return request<{ permissions: string[] }>(`/api/roles/${roleId}/permissions`, {
    method: 'PUT',
    body: JSON.stringify({ permissions }),
  })
}

export function getUserRoles(uid: number) {
  return request<{ builtin_role: string; role_ids: number[] }>(`/api/users/${uid}/roles`)
}

export function assignUserRoles(uid: number, roleIds: number[]) {
  return request<null>(`/api/users/${uid}/roles`, {
    method: 'PUT',
    body: JSON.stringify({ role_ids: roleIds }),
  })
}

// 当前用户的权限列表
export function myPermissions() {
  return request<{ permissions: string[] }>('/api/auth/permissions')
}

export const dataScopeOptions = [
  { value: 'all', label: '全部数据' },
  { value: 'project', label: '所在项目' },
  { value: 'dept', label: '本部门' },
  { value: 'dept_and_sub', label: '本部门及下级' },
  { value: 'self', label: '仅本人' },
]

export function dataScopeLabel(v: string) {
  const o = dataScopeOptions.find((o) => o.value === v)
  return o ? o.label : v
}
