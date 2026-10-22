import { request } from './client'

export interface OverviewStats {
  users: number
  departments: number
  logs: number
  storages: number
  schedule_jobs: number
  cameras?: number
  running_deployments?: number
  pending_events?: number
}

export interface DiskInfo {
  path: string
  total: number
  used: number
  percent: number
}

export interface OverviewSystem {
  hostname: string
  os: string
  platform: string
  arch: string
  uptime_sec: number
  cpu_cores: number
  cpu_percent: number
  mem_total: number
  mem_used: number
  mem_percent: number
  disks: DiskInfo[]
  go_version: string
}

export interface OverviewActivity {
  id: number
  action: string
  username: string
  remark: string
  status: string
  created_at: string
}

export interface OverviewChartPoint {
  label: string
  count: number
  height: number
}

export interface AdminOverview {
  stats: OverviewStats
  system: OverviewSystem
  activity: OverviewActivity[]
  chart: OverviewChartPoint[]
}

export function getAdminOverview() {
  return request<AdminOverview>('/api/admin/overview')
}
