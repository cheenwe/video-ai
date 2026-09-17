import { request } from './client'
import type { PageResult } from './pagination'

export type { PageResult } from './pagination'

export interface SysConfigRow {
  id: number
  category: string
  code: string
  value: string
  status: string
  version: string
  updated_by: number | null
  updated_at: string
}

export function listSysConfigs(params: {
  page?: number
  page_size?: number
  keyword?: string
  category?: string
}) {
  const q = new URLSearchParams()
  if (params.page) q.set('page', String(params.page))
  if (params.page_size) q.set('page_size', String(params.page_size))
  if (params.keyword) q.set('keyword', params.keyword)
  if (params.category) q.set('category', params.category)
  return request<PageResult<SysConfigRow>>(`/api/sys-configs?${q}`)
}

export function createSysConfig(data: {
  category: string
  code: string
  value?: string
  status?: string
  version?: string
}) {
  return request<SysConfigRow>('/api/sys-configs', { method: 'POST', body: JSON.stringify(data) })
}

export function updateSysConfig(
  id: number,
  data: Partial<{ value: string; status: string; version: string }>,
) {
  return request<SysConfigRow>(`/api/sys-configs/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

export function deleteSysConfig(id: number) {
  return request<null>(`/api/sys-configs/${id}`, { method: 'DELETE' })
}
