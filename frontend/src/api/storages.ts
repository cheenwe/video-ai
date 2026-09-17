import { request } from './client'
import type { PageResult } from './pagination'

export interface StorageRow {
  id: string
  name: string
  driver: string
  root_path: string
  enabled: boolean
  sort_order: number
  remark: string
  config: Record<string, unknown> | null
  created_at: string
  updated_at: string
}

export interface StoragePayload {
  name: string
  driver: string
  root_path?: string
  enabled?: boolean
  sort_order?: number
  remark?: string
  config?: Record<string, unknown>
}

export interface DriverInfo {
  driver: string
  name: string
  cloud: boolean
}

export interface ConfigSchemaField {
  key: string
  label: string
  type: 'string' | 'password' | 'int' | 'bool'
  required: boolean
}

export type ConfigSchema = Record<string, ConfigSchemaField[]>

export async function listStorages(params?: {
  page?: number
  page_size?: number
  all?: boolean
}) {
  const q = new URLSearchParams()
  if (params?.all) q.set('all', '1')
  if (params?.page) q.set('page', String(params.page))
  if (params?.page_size) q.set('page_size', String(params.page_size))
  return request<PageResult<StorageRow>>(`/api/storages?${q}`)
}

export async function getStorage(id: string) {
  return request<StorageRow>(`/api/storages/${id}`)
}

export async function createStorage(payload: StoragePayload) {
  return request<StorageRow>('/api/storages', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function updateStorage(id: string, payload: Partial<StoragePayload>) {
  return request<StorageRow>(`/api/storages/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function deleteStorage(id: string) {
  return request<null>(`/api/storages/${id}`, { method: 'DELETE' })
}

export async function testStorage(id: string) {
  return request<{ ok: boolean }>(`/api/storages/${id}/test`, { method: 'POST' })
}

export async function testStorageConfig(
  driver: string,
  config: Record<string, unknown>,
) {
  return request<{ ok: boolean }>('/api/storages/test', {
    method: 'POST',
    body: JSON.stringify({ driver, config }),
  })
}

export async function browseStorage(
  id: string,
  path = '/',
) {
  return request<{ path: string; items: BrowseItem[] }>(
    `/api/storages/${id}/browse?path=${encodeURIComponent(path)}`,
  )
}

export interface BrowseItem {
  name: string
  size: number
  is_dir: boolean
  mod_time: string
}

export async function listDrivers() {
  return request<DriverInfo[]>('/api/storages/drivers')
}

export async function getConfigSchema() {
  return request<ConfigSchema>('/api/storages/config-schema')
}
