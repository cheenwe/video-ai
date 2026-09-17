import { request } from './client'
import type { PageResult } from './pagination'

export type { PageResult } from './pagination'

export interface SysLogRow {
  id: number
  created_at: string
  ip: string
  user_id: number | null
  username: string
  tag: string
  result: string
  action: string
  request_id: string
  remark: string
  source?: number
  source_name?: string
}

export interface SysLogDetail extends SysLogRow {
  before?: Record<string, unknown>
  after?: Record<string, unknown>
}

export type LogCategory = 'log' | 'login' | 'audit' | 'sync' | 'schedule'

export function listSysLogs(params: {
  page?: number
  page_size?: number
  keyword?: string
  tag?: string
  category?: LogCategory
  result?: string
  source?: string
}) {
  const q = new URLSearchParams()
  if (params.page) q.set('page', String(params.page))
  if (params.page_size) q.set('page_size', String(params.page_size))
  if (params.keyword) q.set('keyword', params.keyword)
  if (params.tag) q.set('tag', params.tag)
  if (params.category) q.set('category', params.category)
  if (params.result) q.set('result', params.result)
  if (params.source) q.set('source', params.source)
  return request<PageResult<SysLogRow>>(`/api/sys-logs?${q}`)
}

export function getSysLog(id: number) {
  return request<SysLogDetail>(`/api/sys-logs/${id}`)
}

export function exportSysLogs(params?: {
  keyword?: string
  tag?: string
  result?: string
  time_start?: string
  time_end?: string
}) {
  const q = new URLSearchParams()
  if (params?.keyword) q.set('keyword', params.keyword)
  if (params?.tag) q.set('tag', params.tag)
  if (params?.result) q.set('result', params.result)
  if (params?.time_start) q.set('time_start', params.time_start)
  if (params?.time_end) q.set('time_end', params.time_end)
  const qs = q.toString()
  return `/api/sys-logs/export${qs ? `?${qs}` : ''}`
}
