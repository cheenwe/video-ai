export interface AgentConfig {
  chat_path?: string
  temperature?: number
  max_tokens?: number
}

export interface AgentAccessRule {
  principal_type: 'user' | 'department'
  principal_id: string
}

export interface AgentRow {
  id: string
  slug: string
  name: string
  description?: string
  provider: string
  base_url: string
  api_key?: string
  model: string
  system_prompt?: string
  enabled: boolean
  access_mode?: 'all' | 'restricted'
  access_rules?: AgentAccessRule[]
  sort_order: number
  avatar_url?: string | null
  config?: AgentConfig
}

export interface ChatMessage {
  role: 'user' | 'assistant' | 'system'
  content: string
}

export interface ContextFileRef {
  file_id?: string
  storage_id?: string
  path?: string
}

export interface ChatRequest {
  messages: ChatMessage[]
  stream?: boolean
  context_files?: ContextFileRef[]
  scope?: { storage_id: string; path: string }
}

export interface DocTextMatch {
  filename: string
  path: string
  snippet: string
  score: number
}

export interface AgentDocument {
  id: string
  filename: string
  mime_type?: string
  size: number
  char_count: number
  status: 'ready' | 'failed' | 'parsing'
  error_msg?: string
  storage_id?: string | null
  path?: string
  preview?: string
  created_at: string
}

export interface AttachedFile {
  id: string
  filename: string
  status: AgentDocument['status']
  error_msg?: string
  char_count?: number
}

export type ChatStreamEvent =
  | { type: 'token'; content: string }
  | { type: 'done'; content: string; model: string; role: string; files_used?: string[]; matches?: DocTextMatch[] }
  | { type: 'error'; message: string }
  | { type: 'files_used'; files: string[] }
  | { type: 'matches'; matches: DocTextMatch[] }

export interface AgentPayload {
  slug: string
  name: string
  description?: string
  provider?: string
  base_url: string
  api_key?: string
  model: string
  system_prompt?: string
  enabled?: boolean
  access_mode?: 'all' | 'restricted'
  access_rules?: AgentAccessRule[]
  sort_order?: number
  config?: AgentConfig
}
