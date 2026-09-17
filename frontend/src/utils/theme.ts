export function hexToRgb(hex: string): [number, number, number] | null {
  const h = hex.replace('#', '').trim()
  if (h.length === 3) {
    const r = parseInt(h[0] + h[0], 16)
    const g = parseInt(h[1] + h[1], 16)
    const b = parseInt(h[2] + h[2], 16)
    return [r, g, b]
  }
  if (h.length === 6) {
    const r = parseInt(h.slice(0, 2), 16)
    const g = parseInt(h.slice(2, 4), 16)
    const b = parseInt(h.slice(4, 6), 16)
    if ([r, g, b].some((n) => Number.isNaN(n))) return null
    return [r, g, b]
  }
  return null
}

function clampByte(n: number) {
  return Math.max(0, Math.min(255, Math.round(n)))
}

/** Mix hex color toward white (t>0) or black (t<0). t in [-1, 1]. */
export function mixHex(hex: string, t: number): string {
  const rgb = hexToRgb(hex) || [23, 107, 77]
  const [r, g, b] = rgb
  if (t >= 0) {
    return `#${[r, g, b]
      .map((c) => clampByte(c + (255 - c) * t).toString(16).padStart(2, '0'))
      .join('')}`
  }
  const k = 1 + t
  return `#${[r, g, b]
    .map((c) => clampByte(c * k).toString(16).padStart(2, '0'))
    .join('')}`
}

function rgba(hex: string, a: number) {
  const rgb = hexToRgb(hex) || [23, 107, 77]
  return `rgba(${rgb[0]},${rgb[1]},${rgb[2]},${a})`
}

/** Login hero SVG tinted by system primary color (data URL). */
export function buildLoginHeroBg(accent: string): string {
  const primary = hexToRgb(accent) ? accent : '#176b4d'
  const deep = mixHex(primary, -0.82)
  const mid = mixHex(primary, -0.55)
  const rich = mixHex(primary, -0.28)
  const soft = mixHex(primary, 0.35)
  const soft2 = mixHex(primary, 0.55)
  const soft3 = mixHex(primary, 0.7)

  const svg = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1920 1080" fill="none" preserveAspectRatio="xMidYMid slice">
  <defs>
    <linearGradient id="bg" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="${deep}"/>
      <stop offset="45%" stop-color="${mid}"/>
      <stop offset="100%" stop-color="${rich}"/>
    </linearGradient>
    <linearGradient id="glow" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="${primary}" stop-opacity="0.55"/>
      <stop offset="100%" stop-color="${soft}" stop-opacity="0"/>
    </linearGradient>
    <linearGradient id="beam" x1="0%" y1="0%" x2="100%" y2="0%">
      <stop offset="0%" stop-color="${soft2}" stop-opacity="0"/>
      <stop offset="50%" stop-color="${soft2}" stop-opacity="0.35"/>
      <stop offset="100%" stop-color="${soft2}" stop-opacity="0"/>
    </linearGradient>
    <pattern id="hex" width="56" height="48" patternUnits="userSpaceOnUse" patternTransform="scale(1.2)">
      <path d="M28 2 L52 15 L52 41 L28 54 L4 41 L4 15 Z" stroke="${rgba(soft2, 0.08)}" stroke-width="1" fill="none"/>
    </pattern>
    <pattern id="grid" width="80" height="80" patternUnits="userSpaceOnUse">
      <path d="M 80 0 L 0 0 0 80" fill="none" stroke="rgba(255,255,255,0.04)" stroke-width="1"/>
    </pattern>
    <filter id="blur-lg" x="-50%" y="-50%" width="200%" height="200%">
      <feGaussianBlur stdDeviation="40"/>
    </filter>
    <filter id="glow-sm">
      <feGaussianBlur stdDeviation="4" result="b"/>
      <feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge>
    </filter>
  </defs>
  <rect width="1920" height="1080" fill="url(#bg)"/>
  <rect width="1920" height="1080" fill="url(#hex)"/>
  <rect width="1920" height="1080" fill="url(#grid)" opacity="0.6"/>
  <ellipse cx="1550" cy="280" rx="420" ry="280" fill="url(#glow)" filter="url(#blur-lg)" opacity="0.7"/>
  <ellipse cx="320" cy="780" rx="360" ry="240" fill="${primary}" filter="url(#blur-lg)" opacity="0.18"/>
  <ellipse cx="960" cy="120" rx="500" ry="180" fill="${mixHex(primary, -0.2)}" filter="url(#blur-lg)" opacity="0.12"/>
  <g stroke="${rgba(soft2, 0.2)}" stroke-width="1.5" fill="none">
    <path d="M0 420 H280 L360 360 H620 L700 420 H980 L1060 340 H1320 L1400 400 H1920"/>
    <path d="M0 620 H200 L300 560 H540 L640 640 H900 L1000 580 H1280 L1380 660 H1920"/>
    <path d="M0 820 H160 L260 760 H480 L580 840 H860 L960 780 H1200 L1300 860 H1920"/>
    <path d="M420 0 V180 L500 260 V1080" opacity="0.5"/>
    <path d="M1180 0 V220 L1260 300 V1080" opacity="0.5"/>
    <path d="M1580 0 V160 L1660 240 V1080" opacity="0.35"/>
  </g>
  <rect x="0" y="395" width="1920" height="2" fill="url(#beam)" opacity="0.8"/>
  <rect x="0" y="598" width="1920" height="1.5" fill="url(#beam)" opacity="0.5"/>
  <rect x="0" y="798" width="1920" height="1" fill="url(#beam)" opacity="0.35"/>
  <g filter="url(#glow-sm)">
    <circle cx="360" cy="360" r="5" fill="${soft2}"/>
    <circle cx="700" cy="420" r="4" fill="${soft}"/>
    <circle cx="1060" cy="340" r="6" fill="${soft3}"/>
    <circle cx="1400" cy="400" r="5" fill="${soft2}"/>
    <circle cx="300" cy="560" r="4" fill="${soft}"/>
    <circle cx="640" cy="640" r="5" fill="${soft2}"/>
    <circle cx="1000" cy="580" r="4" fill="${soft}"/>
    <circle cx="1380" cy="660" r="5" fill="${soft3}"/>
    <circle cx="500" cy="260" r="3.5" fill="${soft2}" opacity="0.8"/>
    <circle cx="1260" cy="300" r="3.5" fill="${soft2}" opacity="0.8"/>
  </g>
  <g opacity="0.22">
    <rect x="1420" y="180" width="120" height="280" rx="8" stroke="${rgba(soft3, 0.5)}" stroke-width="1.5" fill="${rgba(primary, 0.15)}"/>
    <rect x="1440" y="210" width="80" height="18" rx="3" fill="${rgba(soft2, 0.25)}"/>
    <rect x="1440" y="242" width="80" height="18" rx="3" fill="${rgba(soft2, 0.18)}"/>
    <rect x="1440" y="274" width="80" height="18" rx="3" fill="${rgba(soft2, 0.22)}"/>
    <rect x="1440" y="306" width="80" height="18" rx="3" fill="${rgba(soft2, 0.15)}"/>
    <rect x="1440" y="338" width="80" height="18" rx="3" fill="${rgba(soft2, 0.2)}"/>
    <rect x="1580" y="240" width="100" height="220" rx="8" stroke="${rgba(soft3, 0.35)}" stroke-width="1.5" fill="${rgba(mixHex(primary, -0.2), 0.12)}"/>
    <rect x="1600" y="268" width="60" height="14" rx="2" fill="${rgba(soft2, 0.2)}"/>
    <rect x="1600" y="294" width="60" height="14" rx="2" fill="${rgba(soft2, 0.15)}"/>
    <rect x="1600" y="320" width="60" height="14" rx="2" fill="${rgba(soft2, 0.18)}"/>
  </g>
  <g stroke="${rgba(soft, 0.28)}" stroke-width="1" fill="none">
    <path d="M80 200 C200 280, 280 320, 420 380"/>
    <path d="M60 320 C180 400, 260 440, 400 500"/>
    <path d="M100 480 C220 560, 300 600, 440 660"/>
    <path d="M140 640 C260 720, 340 760, 480 820"/>
  </g>
  <rect x="0" y="700" width="1920" height="380" fill="url(#bg)" opacity="0.45"/>
  <rect x="0" y="0" width="1920" height="1080" fill="url(#bg)" opacity="0.15"/>
</svg>`

  return `url("data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}")`
}

export function applyTheme(color: string) {
  const rgb = hexToRgb(color)
  const root = document.documentElement
  root.style.setProperty('--tblr-primary', color)
  root.style.setProperty('--cd-primary', color)
  if (rgb) {
    const [r, g, b] = rgb
    root.style.setProperty('--tblr-primary-rgb', `${r}, ${g}, ${b}`)
    root.style.setProperty('--cd-primary-rgb', `${r}, ${g}, ${b}`)
    root.style.setProperty('--cd-primary-soft', `rgba(${r}, ${g}, ${b}, 0.08)`)
    root.style.setProperty('--cd-primary-glow', `rgba(${r}, ${g}, ${b}, 0.25)`)
  }
}

export function applyFavicon(url: string) {
  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }
  link.href = url || '/favicon.svg'
}

export function applyDocumentTitle(name: string) {
  document.title = name
}
