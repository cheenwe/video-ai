import { request } from '@/api/client'
import type { PageResult } from '@/api/pagination'

export interface DepartmentRow {
  id: string
  name: string
  parent_id?: number | null
  sort_order: number
  remark?: string
}

export interface DepartmentPayload {
  name: string
  parent_id?: number | null
  sort_order?: number
  remark?: string
}

export async function listDepartments(params?: { page?: number; page_size?: number; all?: boolean }) {
  const q = new URLSearchParams()
  if (params?.all) q.set('all', '1')
  if (params?.page) q.set('page', String(params.page))
  if (params?.page_size) q.set('page_size', String(params.page_size))
  const data = await request<PageResult<DepartmentRow>>(`/api/departments?${q}`)
  return data
}

export async function listAllDepartments() {
  const data = await listDepartments({ all: true })
  return data.items
}

export async function createDepartment(payload: DepartmentPayload) {
  return request<DepartmentRow>('/api/departments', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function updateDepartment(id: string, payload: Partial<DepartmentPayload>) {
  return request<DepartmentRow>(`/api/departments/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function deleteDepartment(id: string) {
  return request<null>(`/api/departments/${id}`, { method: 'DELETE' })
}
