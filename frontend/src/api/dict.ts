import { request } from './client'

export interface SysDict {
  id: number
  code: string
  name: string
  description?: string
  sort_order: number
  enabled: number
  created_at?: string
  updated_at?: string
}

export interface SysDictItem {
  id: number
  dict_id: number
  code: string
  name: string
  value: string
  sort_order: number
  enabled: number
  remark?: string
  created_at?: string
  updated_at?: string
}

// ---- Admin: 字典类型 ----

export function listDicts(params?: { keyword?: string; page?: number; page_size?: number }) {
  const q = new URLSearchParams()
  if (params?.keyword) q.set('keyword', params.keyword)
  if (params?.page) q.set('page', String(params.page))
  if (params?.page_size) q.set('page_size', String(params.page_size))
  const qs = q.toString()
  return request<{ items: SysDict[]; total: number }>(`/api/dicts${qs ? `?${qs}` : ''}`)
}

export function getDict(id: number) {
  return request<SysDict>(`/api/dicts/${id}`)
}

export function createDict(body: { code: string; name: string; description?: string; sort_order?: number; enabled?: number }) {
  return request<SysDict>('/api/dicts', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function updateDict(id: number, body: { name?: string; description?: string; sort_order?: number; enabled?: number }) {
  return request<SysDict>(`/api/dicts/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

export function deleteDict(id: number) {
  return request<null>(`/api/dicts/${id}`, { method: 'DELETE' })
}

// ---- Admin: 字典项 ----

export function listDictItems(dictId: number) {
  return request<{ items: SysDictItem[]; total: number }>(`/api/dicts/${dictId}/items`)
}

export function createDictItem(dictId: number, body: { code: string; name: string; value: string; sort_order?: number; enabled?: number; remark?: string }) {
  return request<SysDictItem>(`/api/dicts/${dictId}/items`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function updateDictItem(dictId: number, itemId: number, body: { name?: string; value?: string; sort_order?: number; enabled?: number; remark?: string }) {
  return request<SysDictItem>(`/api/dicts/${dictId}/items/${itemId}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

export function deleteDictItem(dictId: number, itemId: number) {
  return request<null>(`/api/dicts/${dictId}/items/${itemId}`, { method: 'DELETE' })
}

// ---- 公开：按编码获取启用的字典项（已认证用户） ----

export function getDictItemsByCode(code: string) {
  return request<{ code: string; name: string; items: SysDictItem[] }>(`/api/dicts/by-code/${encodeURIComponent(code)}/items`)
}
