// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'
import { sanitizeHtml } from './sanitizeHtml'

describe('sanitizeHtml', () => {
  it('removes executable markup', () => {
    const html = sanitizeHtml('<img src=x onerror="alert(1)"><script>alert(2)</script><a href="javascript:alert(3)">x</a>')
    expect(html).not.toContain('onerror')
    expect(html).not.toContain('<script')
    expect(html).not.toContain('javascript:')
  })

  it('keeps ordinary markdown output', () => {
    expect(sanitizeHtml('<p><strong>safe</strong></p>')).toContain('<strong>safe</strong>')
  })
})
