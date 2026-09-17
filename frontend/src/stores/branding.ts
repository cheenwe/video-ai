import { DEFAULT_LOGIN_COPY } from "@/types/loginCopy"
import { publicRequest, request } from '@/api/client'
import type { BrandingSettings } from '@/types/branding'
import { DEFAULT_BRANDING } from '@/types/branding'
import { applyDocumentTitle, applyFavicon, applyTheme } from '@/utils/theme'
import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useBrandingStore = defineStore('branding', () => {
  const settings = ref<BrandingSettings>({ ...DEFAULT_BRANDING })
  const loaded = ref(false)
  const loading = ref(false)

  function apply() {
    applyTheme(settings.value.primary_color)
    applyDocumentTitle(settings.value.site_name)
    applyFavicon(settings.value.favicon_url)
  }

  async function load() {
    if (loading.value) return
    loading.value = true
    try {
      const data = await publicRequest<BrandingSettings>('/api/site/branding')
      settings.value = { ...DEFAULT_BRANDING, ...data, login_copy: {...DEFAULT_LOGIN_COPY, ...data.login_copy} }
      apply()
      loaded.value = true
    } catch {
      settings.value = { ...DEFAULT_BRANDING }
      apply()
    } finally {
      loading.value = false
    }
  }

  async function save(payload: BrandingSettings) {
    const data = await request<BrandingSettings>('/api/site/branding', {
      method: 'PUT',
      body: JSON.stringify(payload),
    })
    settings.value = { ...DEFAULT_BRANDING, ...data, login_copy: {...DEFAULT_LOGIN_COPY, ...data.login_copy} }
    apply()
  }

  return { settings, loaded, loading, load, save, apply }
})
