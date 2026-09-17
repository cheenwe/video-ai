export type QRColorMode = 'solid' | 'gradient' | 'image'

export type QRColorValue = {
  mode: QRColorMode
  color: string
  gradientTo?: string
  imageDataUrl?: string
}

export type QRLabelLayout =
  | 'card'
  | 'tag'
  | 'badge'
  | 'plate'
  | 'poster'
  | 'doc'
  | 'wechat'
  | 'share'
  | 'plain'

export interface QRCodeStyle {
  size?: number
  margin?: number
  errorCorrectionLevel?: 'L' | 'M' | 'Q' | 'H'
  version?: number
  fg?: string
  bg?: string
  fg_fill?: QRColorValue
  bg_fill?: QRColorValue
  accent?: string
  layout?: QRLabelLayout | string
  label_title?: string
  label_subtitle?: string
  /** 标签可编辑字段：key → 用户输入（与 layout schema 对应） */
  fields?: Record<string, string>
  template_id?: string
  template_name?: string
  template_category?: string
  dot_type?: 'square' | 'dots' | 'rounded' | 'extra-rounded' | 'classy' | 'classy-rounded'
  eye_type?: 'square' | 'dot' | 'extra-rounded'
  eye_color_custom?: boolean
  eye_color?: string
  logo_data_url?: string
  show_text?: boolean
  /** url | text */
  content_kind?: 'url' | 'text'
  /** 非 URL 文本时保留原文，便于再编辑 */
  raw_content?: string
}

export interface QRCodeRow {
  id: string
  user_id: number
  name: string
  mode: 'static' | 'dynamic'
  target_url: string
  token?: string
  content: string
  style?: QRCodeStyle
  enabled: boolean
  scan_count: number
  created_at: string
  updated_at: string
}

export type QRCodePayload = {
  name: string
  mode: 'static' | 'dynamic'
  target_url: string
  style?: QRCodeStyle
  enabled?: boolean
}
