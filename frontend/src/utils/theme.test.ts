// @vitest-environment jsdom

import { beforeEach, describe, expect, it } from 'vitest'
import { applyFavicon, applyTheme } from './theme'

describe('brand theme compatibility', () => {
  beforeEach(() => {
    document.documentElement.removeAttribute('style')
    document.head.innerHTML = ''
  })

  it('applies a saved custom primary color instead of forcing the new default', () => {
    applyTheme('#4f46e5')

    expect(document.documentElement.style.getPropertyValue('--cd-primary')).toBe('#4f46e5')
    expect(document.documentElement.style.getPropertyValue('--cd-primary-rgb')).toBe('79, 70, 229')
  })

  it('keeps custom favicon support and falls back to the packaged favicon', () => {
    applyFavicon('data:image/svg+xml,custom')
    expect(document.querySelector<HTMLLinkElement>('link[rel="icon"]')?.href).toContain('data:image/svg+xml,custom')

    applyFavicon('')
    expect(document.querySelector<HTMLLinkElement>('link[rel="icon"]')?.getAttribute('href')).toBe('/favicon.svg')
  })
})
