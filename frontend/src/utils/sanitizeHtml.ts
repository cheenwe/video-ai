import DOMPurify from 'dompurify'
import type { Config } from 'dompurify'

const SAFE_CONFIG: Config = {
  USE_PROFILES: { html: true },
  FORBID_TAGS: ['style', 'script', 'iframe', 'object', 'embed', 'form'],
  FORBID_ATTR: ['style'],
}

export function sanitizeHtml(html: string): string {
  return DOMPurify.sanitize(html, SAFE_CONFIG)
}
