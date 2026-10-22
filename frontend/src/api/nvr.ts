import { request } from './client'

export interface NvrAgent {
  id: number
  name: string
  base_url: string
  token_set: boolean
  enabled: number
  remark: string
  last_status: string
  last_error: string
  last_seen_at: string | null
  version: string
  created_at: string
  updated_at: string
}

export interface NvrAgentPayload {
  name?: string
  base_url?: string
  /** undefined 表示不修改；空字符串表示清除 */
  token?: string
  enabled?: number
  remark?: string
}

export interface AgentNVR {
  id: string
  name: string
  host: string
  http_port: number
  rtsp_port: number
  https: boolean
  insecure_tls: boolean
  username: string
  password: string
  password_env: string
}

export interface AgentRecordConfig {
  enabled: boolean
  stream: string
  segment_minutes: number
  retention_days: number
  audio: boolean
}

export interface AgentCamera {
  id: string
  name: string
  nvr: string
  channel: number
  rtsp_path: string
  url: string
  live_stream: string
  live_transcode: boolean
  record: AgentRecordConfig
}

export interface AgentConfig {
  listen: string
  api_token: string
  ffmpeg: string
  ffprobe: string
  data_dir: string
  nvrs: AgentNVR[]
  cameras: AgentCamera[]
}

export interface ProcessStatus {
  running: boolean
  started_at?: string
  restarts: number
  last_error?: string
  last_exit?: string
}

export interface CameraStatus {
  id: string
  name: string
  nvr?: string
  channel?: number
  live_stream: string
  live_transcode: boolean
  record_stream: string
  record_enabled: boolean
  live: ProcessStatus | null
  record: ProcessStatus | null
}

export interface AgentCameras {
  agent_id: number
  agent_name: string
  online: boolean
  version?: string
  error?: string
  cameras: CameraStatus[]
}

export interface DeviceInfo {
  device_name: string
  model: string
  serial_number: string
  firmware_version: string
}

export interface NvrChannel {
  id: number
  name: string
  ip: string
  protocol: string
  online?: boolean
}

export interface RecordingFile {
  name: string
  size: number
  mod_time: string
  recording: boolean
}

export interface ArchiveSegment {
  start: string
  end: string
  uri: string
}

export interface ArchiveJob {
  camera: string
  start: string
  end: string
  file: string
  running: boolean
  error?: string
  started_at: string
}

const json = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
})

const cam = (agentId: number, camId: string) =>
  `/api/nvr/agents/${agentId}/cameras/${encodeURIComponent(camId)}`

export const listAgents = () => request<{ items: NvrAgent[] }>('/api/nvr/agents')
export const createAgent = (p: NvrAgentPayload) => request<NvrAgent>('/api/nvr/agents', json('POST', p))
export const updateAgent = (id: number, p: NvrAgentPayload) =>
  request<NvrAgent>(`/api/nvr/agents/${id}`, json('PUT', p))
export const deleteAgent = (id: number) => request<null>(`/api/nvr/agents/${id}`, json('DELETE'))

export const testAgentDraft = (p: { id?: number; base_url: string; token?: string | null }) =>
  request<{ version: string; camera_count: number }>('/api/nvr/agents/test', json('POST', p))
export const testAgent = (id: number) =>
  request<{ version: string; camera_count: number }>(`/api/nvr/agents/${id}/test`, json('POST'))

export const getAgentConfig = (id: number) => request<AgentConfig>(`/api/nvr/agents/${id}/config`)
export const saveAgentConfig = (id: number, cfg: AgentConfig) =>
  request<{ ok: boolean; applied: boolean; restart_required: boolean }>(
    `/api/nvr/agents/${id}/config`,
    json('PUT', cfg),
  )
export const debugNVR = (id: number, nvr: AgentNVR) =>
  request<{ device: DeviceInfo; channels: NvrChannel[] }>(`/api/nvr/agents/${id}/debug/nvr`, json('POST', { nvr }))
export const debugStream = (id: number, p: { nvr: AgentNVR; channel: number; stream: string; rtsp_path?: string }) =>
  request<{ result: string }>(`/api/nvr/agents/${id}/debug/stream`, json('POST', p))

export const listCameras = () => request<{ items: AgentCameras[] }>('/api/nvr/cameras')
export const recordControl = (agentId: number, camId: string, action: 'start' | 'stop') =>
  request<{ ok: boolean }>(`${cam(agentId, camId)}/record/${action}`, json('POST'))
export const listRecordings = (agentId: number, camId: string) =>
  request<RecordingFile[] | null>(`${cam(agentId, camId)}/recordings`)
export const searchArchive = (agentId: number, camId: string, start: string, end: string) =>
  request<ArchiveSegment[] | null>(
    `${cam(agentId, camId)}/archive/search?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`,
  )
export const archiveStatus = (agentId: number, camId: string) =>
  request<ArchiveJob | null>(`${cam(agentId, camId)}/archive`)
export const startArchive = (agentId: number, camId: string, start: string, end: string) =>
  request<ArchiveJob>(`${cam(agentId, camId)}/archive`, json('POST', { start, end }))

export const liveURL = (agentId: number, camId: string) =>
  `/api/nvr/agents/${agentId}/live/${encodeURIComponent(camId)}/index.m3u8`
export const recordingURL = (agentId: number, camId: string, file: string, download = false) =>
  `/api/nvr/agents/${agentId}/recordings/${encodeURIComponent(camId)}/${encodeURIComponent(file)}${download ? '?download=1' : ''}`
