import { request } from './client'

export interface Notification {
  id: number
  user_id: number
  type: string
  title: string
  content: string
  level: string
  is_read: number
  read_at?: string
  ref_type?: string
  ref_id?: string
  created_at?: string
  updated_at?: string
}

// ---- 站内通知（当前用户） ----

export function listNotifications(params?: { is_read?: string; page?: number; page_size?: number }) {
  const q = new URLSearchParams()
  if (params?.is_read !== undefined) q.set('is_read', params.is_read)
  if (params?.page) q.set('page', String(params.page))
  if (params?.page_size) q.set('page_size', String(params.page_size))
  const qs = q.toString()
  return request<{ items: Notification[]; total: number }>(`/api/notifications${qs ? `?${qs}` : ''}`)
}

export function getUnreadCount() {
  return request<{ count: number }>('/api/notifications/unread-count')
}

export function markNotificationRead(id: number) {
  return request<null>(`/api/notifications/${id}/read`, { method: 'PUT' })
}

export function markAllNotificationsRead() {
  return request<null>('/api/notifications/read-all', { method: 'PUT' })
}

export function deleteNotification(id: number) {
  return request<null>(`/api/notifications/${id}`, { method: 'DELETE' })
}

// ---- Webhook 管理 ----

export interface WebhookEndpoint {
  id: number
  name: string
  url: string
  event_types: string[]
  enabled: number
  timeout_sec: number
  max_retries: number
  created_at?: string
  updated_at?: string
}

export interface WebhookDelivery {
  id: number
  endpoint_id: number
  delivery_id: string
  event_type: string
  payload: string
  status_code: number
  response_body: string
  status: string
  retry_count: number
  next_retry_at?: string
  created_at?: string
  updated_at?: string
}

export function listWebhooks() {
  return request<{ items: WebhookEndpoint[] }>('/api/admin/webhooks')
}

export function createWebhook(body: {
  name: string
  url: string
  secret?: string
  event_types?: string[]
  enabled?: number
  timeout_sec?: number
  max_retries?: number
}) {
  return request<{ id: number }>('/api/webhooks', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function updateWebhook(id: number, body: Record<string, unknown>) {
  return request<null>(`/api/webhooks/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

export function deleteWebhook(id: number) {
  return request<null>(`/api/webhooks/${id}`, { method: 'DELETE' })
}

export function listWebhookDeliveries(id: number, params?: { status?: string; page?: number; page_size?: number }) {
  const q = new URLSearchParams()
  if (params?.status) q.set('status', params.status)
  if (params?.page) q.set('page', String(params.page))
  if (params?.page_size) q.set('page_size', String(params.page_size))
  const qs = q.toString()
  return request<{ items: WebhookDelivery[]; total: number }>(`/api/webhooks/${id}/deliveries${qs ? `?${qs}` : ''}`)
}

export function retryWebhookDelivery(id: number) {
  return request<null>(`/api/webhooks/deliveries/${id}/retry`, { method: 'POST' })
}
