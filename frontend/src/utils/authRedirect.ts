const REDIRECT_STORAGE_KEY = 'baseplatform_login_redirect'

/** Only allow same-origin relative paths (block //evil.com open redirects). */
export function safeInternalPath(raw: unknown): string | null {
  if (typeof raw !== 'string') return null
  const path = raw.trim()
  if (!path.startsWith('/') || path.startsWith('//') || /[\\\x00-\x1f]/.test(path)) return null
  if (path.startsWith('/login')) return null
  return path
}

export function rememberLoginRedirect(fullPath: string) {
  const path = safeInternalPath(fullPath)
  if (!path) return
  try {
    sessionStorage.setItem(REDIRECT_STORAGE_KEY, path)
  } catch {
    /* ignore quota / private mode */
  }
}

export function consumeLoginRedirect(): string | null {
  try {
    const path = safeInternalPath(sessionStorage.getItem(REDIRECT_STORAGE_KEY))
    sessionStorage.removeItem(REDIRECT_STORAGE_KEY)
    return path
  } catch {
    return null
  }
}

export function resolvePostLoginPath(queryRedirect?: unknown): string {
  const fromQuery = safeInternalPath(queryRedirect)
  const fromStore = consumeLoginRedirect()
  return fromQuery || fromStore || '/admin/overview'
}

export function loginLocationWithRedirect(fullPath?: string): string {
  const path = safeInternalPath(fullPath || `${window.location.pathname}${window.location.search}${window.location.hash}`)
  if (!path) return '/login'
  return `/login?redirect=${encodeURIComponent(path)}`
}
