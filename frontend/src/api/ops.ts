import { ApiError, getToken, request } from './client'

export interface BackupManifest {
  id: string
  created_at: string
  archive_path: string
  size_bytes: number
  data_home: string
  note: string
  db_driver?: string
  db_dump_name?: string
  db_dump_bytes?: number
  db_dump_error?: string
}

export interface BackupStatus {
  restore_path?: string
  state: string
  phase: string
  percent: number
  message: string
  error: string
  backup_id: string
}

export interface NTPQueryResult {
  ntp_time: string
  local_time: string
  offset_ms: number
  round_trip_ms: number
  server: string
}

export interface GeneralSettings {
  page_size: number
  local_files_root: string
  ntp_enabled: boolean
  ntp_server: string
}

export function listBackups() {
  return request<{ items: BackupManifest[] }>('/api/admin/ops/backups')
}

export function backupStatus() {
  return request<BackupStatus>('/api/admin/ops/backups/status')
}

export function createBackup() {
  return request<BackupManifest>('/api/admin/ops/backups', { method: 'POST' })
}

export function deleteBackup(id: string) {
  return request<null>(`/api/admin/ops/backups/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export function restoreBackup(id: string) {
  return request<BackupStatus>(
    `/api/admin/ops/backups/${encodeURIComponent(id)}/restore`,
    { method: 'POST' },
  )
}

export function queryNTP(server?: string) {
  const q = new URLSearchParams()
  if (server) q.set('server', server)
  const qs = q.toString()
  return request<NTPQueryResult>(`/api/admin/ops/ntp${qs ? '?' + qs : ''}`)
}

export function getGeneralSettings() {
  return request<GeneralSettings>('/api/admin/settings/general')
}

async function authorizedDownload(path: string, filename: string) {
  const token = getToken()
  const res = await fetch(path, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    credentials: 'same-origin',
  })
  if (!res.ok) throw new ApiError('下载失败', res.status)
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

export function downloadBackup(id: string) {
  return authorizedDownload(
    `/api/admin/ops/backups/${encodeURIComponent(id)}/download`,
    `baseplatform-backup-${id}.tar.gz`,
  )
}
