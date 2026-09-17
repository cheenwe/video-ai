<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useLicenseStore } from '@/stores/license'
import { useAuthStore } from '@/stores/auth'
import { useBrandingStore } from '@/stores/branding'

const route = useRoute()
const licenseStore = useLicenseStore()
const auth = useAuthStore()
const branding = useBrandingStore()

const wm = computed(() => licenseStore.watermark)
const show = computed(() => {
  const w = wm.value
  if (!w?.enabled || !licenseStore.hasFeature('watermark')) return false
  // 视频AI平台：app 范围覆盖登录后的所有页面；preview 覆盖预览页
  return w.scope?.some((s) => ['app', 'preview', 'share', 'download'].includes(s)) ?? false
})

function escapeXml(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}

function buildWatermarkPattern(
  text: string,
  fontSize: number,
  angle: number,
  opacity: number,
  density: number,
) {
  const safe = escapeXml(text)
  const d = Math.min(2, Math.max(0.1, density ?? 0.7))
  const scale = 1 / d
  const tileW = Math.min(720, Math.max(220, Math.round((text.length * fontSize * 0.55 + 80) * scale)))
  const tileH = Math.max(100, Math.round(fontSize * 3.5 * scale))
  const cx = tileW / 2
  const cy = tileH / 2
  const alpha = Math.min(0.5, Math.max(0.06, opacity))
  const svg = [
    `<svg xmlns="http://www.w3.org/2000/svg" width="${tileW}" height="${tileH}">`,
    `<text x="${cx}" y="${cy}" dominant-baseline="middle" text-anchor="middle"`,
    ` fill="rgba(100,116,139,${alpha})" font-size="${fontSize}"`,
    ` font-family="system-ui,-apple-system,'Segoe UI',sans-serif"`,
    ` transform="rotate(${angle} ${cx} ${cy})">${safe}</text>`,
    '</svg>',
  ].join('')
  return {
    backgroundImage: `url("data:image/svg+xml,${encodeURIComponent(svg)}")`,
    backgroundRepeat: 'repeat',
    backgroundSize: `${tileW}px ${tileH}px`,
  }
}

const style = computed(() => {
  const w = wm.value
  if (!w) return {}
  let text = w.text || '{username} · {datetime}'
  const user = auth.user?.real_name || auth.user?.username || branding.settings.site_name || '用户'
  text = text
    .replaceAll('{username}', user)
    .replaceAll('{datetime}', new Date().toLocaleString('zh-CN', { hour12: false }))
  const angle = w.angle ?? -25
  const opacity = w.opacity ?? 0.15
  const size = w.font_size ?? 14
  const density = w.density ?? 0.7
  return buildWatermarkPattern(text, size, angle, opacity, density)
})
</script>

<template>
  <div v-if="show" class="va-watermark" :style="style" aria-hidden="true" />
</template>

<style scoped>
.va-watermark {
  position: fixed;
  inset: 0;
  pointer-events: none;
  z-index: 9998;
}
</style>
