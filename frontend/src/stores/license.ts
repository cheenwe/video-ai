import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import * as licenseApi from '@/api/license'
import type { LicenseMeta, SiteInfo, WatermarkInfo } from '@/api/license'
import { normalizeWatermarkInfo } from '@/api/license'

declare const __APP_VERSION__: string

const EDITION_LABELS: Record<string, string> = {
  community: '社区版',
  flagship: '旗舰版',
}

export const useLicenseStore = defineStore('license', () => {
  const siteInfo = ref<SiteInfo | null>(null)
  const loading = ref(false)

  const version = computed(() => siteInfo.value?.version || __APP_VERSION__ || 'dev')
  const edition = computed(() => siteInfo.value?.edition || 'community')
  const license = computed<LicenseMeta>(() => siteInfo.value?.license || {
    edition: 'community',
    valid: true,
    features: [],
  })
  const watermark = computed<WatermarkInfo | null>(() => normalizeWatermarkInfo(siteInfo.value?.watermark))

  const isCommunity = computed(() => edition.value === 'community')
  const isFlagship = computed(() => edition.value === 'flagship' && license.value.valid)

  const editionLabel = computed(() => EDITION_LABELS[edition.value] || '社区版')

  const editionBadgeClass = computed(() => {
    switch (edition.value) {
      case 'flagship':
        return 'bg-primary-lt text-primary'
      default:
        return 'bg-secondary-lt text-secondary'
    }
  })

  const maxUsers = computed(() => siteInfo.value?.max_users ?? license.value.max_users ?? 0)
  const userCount = computed(() => siteInfo.value?.user_count ?? license.value.user_count ?? 0)

  const maxUsersLabel = computed(() => (maxUsers.value === 0 ? '不限' : String(maxUsers.value)))

  const atUserLimit = computed(() => maxUsers.value > 0 && userCount.value >= maxUsers.value)

  function hasFeature(code: string) {
    return license.value.valid && (license.value.features?.includes(code) ?? false)
  }

  async function load() {
    loading.value = true
    try {
      siteInfo.value = await licenseApi.getSiteInfo()
    } catch {
      siteInfo.value = {
        version: __APP_VERSION__ || 'dev',
        edition: 'community',
        max_users: 5,
        user_count: 0,
        license: { edition: 'community', valid: true, features: [], max_users: 5 },
      }
    } finally {
      loading.value = false
    }
  }

  async function refreshLicense() {
    siteInfo.value = await licenseApi.getSiteInfo()
  }

  return {
    siteInfo,
    loading,
    version,
    edition,
    license,
    watermark,
    isCommunity,
    isFlagship,
    editionLabel,
    editionBadgeClass,
    maxUsers,
    userCount,
    maxUsersLabel,
    atUserLimit,
    hasFeature,
    load,
    refreshLicense,
  }
})
