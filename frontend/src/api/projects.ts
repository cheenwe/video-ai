import { request } from './client'

export interface Project {
  id: number
  code: string
  name: string
  description?: string
  owner_id?: number
  enabled: boolean
  created_at?: string
  updated_at?: string
}

export interface ProjectMember {
  id: number
  project_id: number
  user_id: number
  username: string
  real_name?: string
  role: string
  created_at?: string
}

export function listProjects(params?: { keyword?: string; page?: number; page_size?: number }) {
  const q = new URLSearchParams()
  if (params?.keyword) q.set('keyword', params.keyword)
  if (params?.page) q.set('page', String(params.page))
  if (params?.page_size) q.set('page_size', String(params.page_size))
  const qs = q.toString()
  return request<{ items: Project[]; total: number; page: number; page_size: number }>(
    `/api/projects${qs ? `?${qs}` : ''}`,
  )
}

export function createProject(body: { code: string; name: string; description?: string; owner_id?: number }) {
  return request<Project>('/api/projects', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function updateProject(id: number, body: { name?: string; description?: string; owner_id?: number; enabled?: number }) {
  return request<Project>(`/api/projects/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

export function deleteProject(id: number) {
  return request<null>(`/api/projects/${id}`, { method: 'DELETE' })
}

export function listProjectMembers(projectId: number) {
  return request<{ items: ProjectMember[] }>(`/api/projects/${projectId}/members`)
}

export function addProjectMember(projectId: number, body: { user_id: number; role?: string }) {
  return request<{ id: number }>(`/api/projects/${projectId}/members`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function updateProjectMember(memberId: number, role: string) {
  return request<null>(`/api/projects/members/${memberId}`, {
    method: 'PUT',
    body: JSON.stringify({ role }),
  })
}

export function removeProjectMember(memberId: number) {
  return request<null>(`/api/projects/members/${memberId}`, { method: 'DELETE' })
}

export const memberRoleOptions = [
  { value: 'owner', label: '负责人' },
  { value: 'admin', label: '管理员' },
  { value: 'member', label: '成员' },
]

export function memberRoleLabel(v: string) {
  const o = memberRoleOptions.find((o) => o.value === v)
  return o ? o.label : v
}
