import { request } from './client'

export interface ScheduleCommand {
  id: number
  name: string
  key: string
  kind: string
  bin: string
  args_template: string
  workdir: string
  timeout_sec: number
  enabled: number
  description: string
  created_at?: string
  updated_at?: string
}

export interface ScheduleJob {
  id: number
  name: string
  job_type: string
  builtin_key: string
  command_id: number | null
  command_name?: string
  command_key?: string
  args: string
  cron: string
  enabled: number
  options: string
  is_builtin: number
  last_run_at?: string | null
  last_status: string
  last_error: string
  last_duration_ms: number
  run_count: number
  fail_count: number
  next_run_at?: string | null
  created_at?: string
  updated_at?: string
}

export interface ScheduleStats {
  total: number
  enabled: number
  failed: number
  running: number
  jobs: ScheduleJob[]
}

export function listJobs() {
  return request<{ items: ScheduleJob[] }>('/api/admin/schedule/jobs')
}

export function getJob(id: number) {
  return request<ScheduleJob>(`/api/admin/schedule/jobs/${id}`)
}

export function createJob(body: Record<string, unknown>) {
  return request<ScheduleJob>('/api/admin/schedule/jobs', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function updateJob(id: number, body: Record<string, unknown>) {
  return request<ScheduleJob>(`/api/admin/schedule/jobs/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

export function deleteJob(id: number) {
  return request<null>(`/api/admin/schedule/jobs/${id}`, { method: 'DELETE' })
}

export function runJob(id: number) {
  return request<null>(`/api/admin/schedule/jobs/${id}/run`, {
    method: 'POST',
  })
}

export function listCommands() {
  return request<{ items: ScheduleCommand[] }>('/api/admin/schedule/commands')
}

export function createCommand(body: Record<string, unknown>) {
  return request<ScheduleCommand>('/api/admin/schedule/commands', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function updateCommand(id: number, body: Record<string, unknown>) {
  return request<ScheduleCommand>(`/api/admin/schedule/commands/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

export function deleteCommand(id: number) {
  return request<null>(`/api/admin/schedule/commands/${id}`, {
    method: 'DELETE',
  })
}

export function getStats() {
  return request<ScheduleStats>('/api/admin/schedule/stats')
}

export function validateCron(cron: string) {
  return request<{ valid: boolean }>('/api/admin/schedule/validate-cron', {
    method: 'POST',
    body: JSON.stringify({ cron }),
  })
}
