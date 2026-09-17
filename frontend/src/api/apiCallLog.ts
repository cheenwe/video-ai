import { request } from './client'

export interface ApiCallLogRow {
  id: number
  token_id: number
  user_id: number
  method: string
  path: string
  status_code: number
  duration_ms: number
  ip: string
  user_agent: string
  created_at: string
}

export interface ApiCallLogListParams {
  token_id?: number
  user_id?: number
  method?: string
  status_code?: number
  path?: string
  time_start?: string
  time_end?: string
  page?: number
  page_size?: number
}

// 管理员：查看全部 API 调用日志
export function listApiCallLogs(params?: ApiCallLogListParams) {
  const q = new URLSearchParams()
  if (params?.token_id) q.set('token_id', String(params.token_id))
  if (params?.user_id) q.set('user_id', String(params.user_id))
  if (params?.method) q.set('method', params.method)
  if (params?.status_code) q.set('status_code', String(params.status_code))
  if (params?.path) q.set('path', params.path)
  if (params?.time_start) q.set('time_start', params.time_start)
  if (params?.time_end) q.set('time_end', params.time_end)
  if (params?.page) q.set('page', String(params.page))
  if (params?.page_size) q.set('page_size', String(params.page_size))
  const qs = q.toString()
  return request<{ items: ApiCallLogRow[]; total: number; page: number; page_size: number }>(
    `/api/admin/api-call-logs${qs ? `?${qs}` : ''}`,
  )
}

// 当前用户：查看自己某个令牌的调用日志
export function listTokenCallLogs(tokenId: number, params?: ApiCallLogListParams) {
  const q = new URLSearchParams()
  if (params?.method) q.set('method', params.method)
  if (params?.status_code) q.set('status_code', String(params.status_code))
  if (params?.path) q.set('path', params.path)
  if (params?.time_start) q.set('time_start', params.time_start)
  if (params?.time_end) q.set('time_end', params.time_end)
  if (params?.page) q.set('page', String(params.page))
  if (params?.page_size) q.set('page_size', String(params.page_size))
  const qs = q.toString()
  return request<{ items: ApiCallLogRow[]; total: number; page: number; page_size: number }>(
    `/api/tokens/${tokenId}/call-logs${qs ? `?${qs}` : ''}`,
  )
}
