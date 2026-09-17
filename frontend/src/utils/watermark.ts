import type { WatermarkInfo } from '@/api/license'
import { normalizeWatermarkInfo } from '@/api/license'

export const DEFAULT_WATERMARK = {
  enabled: false,
  text: '{username} · {datetime}',
  opacity: 0.15,
  font_size: 14,
  angle: -25,
  density: 0.7,
  scope: ['app', 'preview', 'share', 'download'],
}

export function normalizeWatermarkSettings(input: Partial<typeof DEFAULT_WATERMARK>) {
  const merged = { ...DEFAULT_WATERMARK, ...input }
  return normalizeWatermarkInfo(merged) || { ...DEFAULT_WATERMARK }
}
