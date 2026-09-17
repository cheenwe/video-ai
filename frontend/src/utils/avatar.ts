export type AvatarKind = 'user' | 'agent'

const USER_PALETTE = [
  { bg: '#eef2ff', fg: '#4f46e5' },
  { bg: '#ecfdf5', fg: '#059669' },
  { bg: '#fff7ed', fg: '#ea580c' },
  { bg: '#fef2f2', fg: '#dc2626' },
  { bg: '#f5f3ff', fg: '#7c3aed' },
  { bg: '#ecfeff', fg: '#0891b2' },
]

const AGENT_FALLBACK = { bg: '#f3e8ff', fg: '#7c3aed' }

export function avatarInitials(name: string) {
  const n = (name || '').trim()
  return n ? n.slice(0, 1).toUpperCase() : '?'
}

export function avatarColors(name: string, kind: AvatarKind = 'user') {
  if (kind === 'agent') return AGENT_FALLBACK
  const seed = (name || '').trim() || kind
  let hash = 0
  for (let i = 0; i < seed.length; i++) {
    hash = seed.charCodeAt(i) + ((hash << 5) - hash)
  }
  return USER_PALETTE[Math.abs(hash) % USER_PALETTE.length]
}

export function hasCustomAvatar(avatarUrl: string | null | undefined) {
  return !!avatarUrl?.trim()
}
