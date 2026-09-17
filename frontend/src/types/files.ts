export interface StorageConfig {
  base_path?: string
  endpoint?: string
  bucket?: string
  access_key?: string
  secret_key?: string
  region?: string
  use_ssl?: boolean
  prefix?: string
  host?: string
  port?: number
  username?: string
  password?: string
  url?: string
  server?: string
  export?: string
  mount_point?: string
  sub_path?: string
  version?: number
  options?: string
  share?: string
  domain?: string
  remote_path?: string
  connected?: boolean
  account_name?: string
  token_expiry?: string
  oauth_scope?: string
}

export interface StorageRow {
  id: string
  name: string
  driver: string
  root_path: string
  access_root_path?: string
  enabled: boolean
  sort_order?: number
  remark?: string
  config?: StorageConfig
}

export interface FileEntry {
  id: string
  name: string
  path: string
  isDir: boolean
  size: number
  modified: string
  mimeType?: string
  /** 列表项能力：是否可预览（独立于下载） */
  canPreview?: boolean
  /** 列表项能力：是否可下载/读取 */
  canRead?: boolean
  createdAt?: string
  modifiedAt?: string
  createdBy?: number
  modifiedBy?: number
  createdSource?: number
  modifiedSource?: number
  createdByName?: string
  modifiedByName?: string
  createdSourceName?: string
  modifiedSourceName?: string
}
