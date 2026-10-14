<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import QRCode from 'qrcode'
import {
  IconCertificate,
  IconRefresh,
  IconUpload,
  IconScan,
  IconShieldCheck,
  IconTrash,
  IconGift,
  IconUsers,
  IconBuilding,
  IconCalendar,
  IconCopy,
  IconCircleCheck,
  IconAdjustments,
} from '@tabler/icons-vue'
import CdModal from '@/components/CdModal.vue'
import { ApiError } from '@/api/client'
import * as licenseApi from '@/api/license'
import * as settingsApi from '@/api/settings'
import { useLicenseStore } from '@/stores/license'
import { useToast } from '@/composables/useToast'

interface LicenseSettings {
  license_server_url: string
  heartbeat_hours: number
  auto_sync_crl: boolean
  crl_sync_hours: number
}

async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

const licenseStore = useLicenseStore()
const toast = useToast()

const licenseCfg = ref<LicenseSettings>({
  license_server_url: 'https://license.nextfile.cn',
  heartbeat_hours: 24,
  auto_sync_crl: true,
  crl_sync_hours: 12,
})
const activateId = ref('')
const machineFp = ref<licenseApi.MachineFingerprint | null>(null)
const removing = ref(false)
const applyingTrial = ref(false)
const activatingTrial = ref(false)
const trialStatus = ref<licenseApi.TrialStatus | null>(null)
const trialForm = ref({
  team_name: '',
  contact: '',
  edition: 'enterprise' as 'team' | 'enterprise',
})
const fileInput = ref<HTMLInputElement | null>(null)
const fpQrCanvas = ref<HTMLCanvasElement | null>(null)
const showAdvancedCfg = ref(false)
const showUpdateLicense = ref(false)
const savingAdvancedCfg = ref(false)
const syncingLicense = ref(false)
const advancedCfgDraft = ref<LicenseSettings>({
  license_server_url: 'https://license.nextfile.cn',
  heartbeat_hours: 24,
  auto_sync_crl: true,
  crl_sync_hours: 12,
})

const DEFAULT_LICENSE_SERVER = 'https://license.nextfile.cn'

const featureLabels: Record<string, string> = {
  branding_custom: '高级品牌个性化',
  watermark: '系统水印',
  gateway: '对外网关服务',
  storage_sync: '存储同步',
  admin_totp: '管理员双因素认证',
  sso: 'LDAP / SSO',
  ops: '运维中心',
  extensions: '扩展应用平台',
  online_upgrade: '在线升级',
}

const licensedFeatures = computed(() =>
  Object.entries(featureLabels).map(([code, label]) => ({
    code,
    label,
    enabled: licenseStore.hasFeature(code),
  })),
)

const enabledFeatureCount = computed(
  () => licensedFeatures.value.filter((f) => f.enabled).length,
)

const displayShortId = computed(
  () => machineFp.value?.short_machine_id || machineFp.value?.machine_id || '',
)

const hasActiveLicense = computed(
  () => !!licenseStore.license.license_id || licenseStore.edition !== 'community',
)

const canApplyTrial = computed(
  () =>
    licenseStore.isCommunity &&
    (!trialStatus.value || trialStatus.value.status === 'none'),
)

const showTrialSection = computed(
  () =>
    licenseStore.isCommunity ||
    trialStatus.value?.status === 'approved' ||
    trialStatus.value?.status === 'rejected',
)

const trialEditionLabel = (edition?: string) => {
  switch (edition) {
    case 'team':
      return '团队版'
    case 'enterprise':
    case 'flagship':
      return '旗舰版'
    default:
      return '社区版'
  }
}

const trialStatusBadge = computed(() => {
  const s = trialStatus.value?.status
  if (s === 'pending') return { text: '待审批', class: 'bg-yellow-lt text-yellow' }
  if (s === 'approved') return { text: '已通过', class: 'bg-green-lt text-green' }
  if (s === 'rejected') return { text: '未通过', class: 'bg-danger-lt text-danger' }
  return null
})

const editionHeroClass = computed(() => {
  switch (licenseStore.edition) {
    case 'enterprise':
      return 'cd-license__block--enterprise'
    default:
      return 'cd-license__block--community'
  }
})

const licenseStatus = computed(() => {
  if (!licenseStore.license.valid) {
    return { text: '授权无效', class: 'bg-danger-lt text-danger' }
  }
  if (hasActiveLicense.value) {
    return { text: '授权有效', class: 'bg-green-lt text-green' }
  }
  return { text: '未激活付费授权', class: 'bg-secondary-lt text-secondary' }
})

const seatUsagePercent = computed(() => {
  const max = licenseStore.maxUsers
  if (!max || max <= 0) return null
  return Math.min(100, Math.round((licenseStore.userCount / max) * 100))
})

const seatUsageTone = computed(() => {
  const p = seatUsagePercent.value
  if (p == null) return 'ok'
  if (p >= 90) return 'danger'
  if (p >= 70) return 'warn'
  return 'ok'
})

function formatExpiresAt(raw?: string) {
  if (!raw) return '—'
  const d = new Date(raw)
  if (Number.isNaN(d.getTime())) return raw
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

async function renderFingerprintQR(text: string) {
  await nextTick()
  if (!fpQrCanvas.value || !text) return
  await QRCode.toCanvas(fpQrCanvas.value, text, { width: 108, margin: 1 })
}

async function load() {
  const cfg = await settingsApi.getSettings<LicenseSettings>('license')
  if (!cfg.license_server_url?.trim()) {
    cfg.license_server_url = DEFAULT_LICENSE_SERVER
  }
  licenseCfg.value = cfg
  try {
    machineFp.value = await licenseApi.getMachineFingerprint()
  } catch {
    machineFp.value = null
  }
  await renderFingerprintQR(displayShortId.value)
  await loadTrialStatus()
}

async function loadTrialStatus() {
  if (!licenseCfg.value.license_server_url.trim()) {
    trialStatus.value = null
    return
  }
  try {
    trialStatus.value = await licenseApi.getTrialStatus(licenseCfg.value.license_server_url)
  } catch {
    trialStatus.value = null
  }
}

watch(displayShortId, (id) => {
  void renderFingerprintQR(id)
})

async function saveLicenseCfg() {
  await settingsApi.saveSettings('license', licenseCfg.value)
  toast.show('授权服务器配置已保存')
  await loadTrialStatus()
}

function openAdvancedCfg() {
  advancedCfgDraft.value = {
    ...licenseCfg.value,
    license_server_url: licenseCfg.value.license_server_url?.trim() || DEFAULT_LICENSE_SERVER,
  }
  showAdvancedCfg.value = true
}

async function saveAdvancedCfg() {
  savingAdvancedCfg.value = true
  try {
    licenseCfg.value = {
      ...licenseCfg.value,
      license_server_url:
        advancedCfgDraft.value.license_server_url.trim() || DEFAULT_LICENSE_SERVER,
    }
    await saveLicenseCfg()
    showAdvancedCfg.value = false
  } catch (err) {
    toast.show(err instanceof Error ? err.message : '保存失败')
  } finally {
    savingAdvancedCfg.value = false
  }
}

async function onApplyTrial() {
  const team = trialForm.value.team_name.trim()
  const contact = trialForm.value.contact.trim()
  if (!team && !contact) {
    toast.show('请填写团队/企业名称或联系邮箱')
    return
  }
  if (!licenseCfg.value.license_server_url.trim()) {
    toast.show('请先配置并保存授权服务器地址')
    return
  }
  applyingTrial.value = true
  try {
    trialStatus.value = await licenseApi.applyTrialLicense(licenseCfg.value.license_server_url, {
      team_name: team,
      contact,
      edition: trialForm.value.edition,
    })
    if (trialStatus.value.status === 'approved') {
      toast.show('试用已通过，请激活试用授权')
    } else if (trialStatus.value.status === 'rejected') {
      toast.show(trialStatus.value.message || '该机器已申请过试用')
      await loadTrialStatus()
    } else {
      toast.show(trialStatus.value.message || '试用申请已提交')
    }
  } catch (err) {
    toast.show(err instanceof Error ? err.message : '申请失败')
  } finally {
    applyingTrial.value = false
  }
}

async function onActivateTrial() {
  const id = trialStatus.value?.license_id
  if (!id) return
  activatingTrial.value = true
  try {
    await licenseApi.activateLicense(licenseCfg.value.license_server_url, id)
    await licenseStore.refreshLicense()
    toast.show('试用授权激活成功')
  } catch (err) {
    toast.show(err instanceof Error ? err.message : '激活失败')
  } finally {
    activatingTrial.value = false
  }
}

async function onImportFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    await licenseApi.importLicenseFile(file)
    await licenseStore.refreshLicense()
    showUpdateLicense.value = false
    toast.show('授权导入成功')
  } catch (err) {
    toast.show(err instanceof Error ? err.message : '导入失败')
  }
  input.value = ''
}

async function onActivate() {
  if (!activateId.value.trim()) return toast.show('请填写 License ID')
  try {
    await licenseApi.activateLicense(licenseCfg.value.license_server_url, activateId.value.trim())
    await licenseStore.refreshLicense()
    showUpdateLicense.value = false
    toast.show('在线激活成功')
  } catch (err) {
    toast.show(err instanceof Error ? err.message : '激活失败')
  }
}

async function onSyncLicenseInfo() {
  const url = (
    showAdvancedCfg.value
      ? advancedCfgDraft.value.license_server_url
      : licenseCfg.value.license_server_url
  ).trim() || DEFAULT_LICENSE_SERVER
  syncingLicense.value = true
  try {
    if (url !== licenseCfg.value.license_server_url) {
      licenseCfg.value = { ...licenseCfg.value, license_server_url: url }
      await settingsApi.saveSettings('license', licenseCfg.value)
      if (showAdvancedCfg.value) {
        advancedCfgDraft.value = { ...advancedCfgDraft.value, license_server_url: url }
      }
    }
    const { message } = await licenseApi.syncLicenseInfo()
    await licenseStore.refreshLicense()
    await loadTrialStatus()
    toast.show(message)
  } catch (err) {
    toast.show(err instanceof ApiError ? err.message : err instanceof Error ? err.message : '同步失败')
  } finally {
    syncingLicense.value = false
  }
}

async function onRemoveLicense() {
  if (!hasActiveLicense.value) return
  if (
    !confirm(
      '确定移除当前授权？移除后将回退到社区版，部分功能与席位限制会立即生效。',
    )
  ) {
    return
  }
  removing.value = true
  try {
    await licenseApi.removeLicense()
    await licenseStore.refreshLicense()
    activateId.value = ''
    showAdvancedCfg.value = false
    toast.show('授权已移除')
  } catch (err) {
    toast.show(err instanceof Error ? err.message : '移除失败')
  } finally {
    removing.value = false
  }
}

async function copyFingerprint() {
  const id = displayShortId.value
  if (!id) return
  const ok = await copyToClipboard(id)
  toast.show(ok ? '部署指纹已复制' : '复制失败，请手动选择复制')
}

async function copyLicenseId() {
  const id = licenseStore.license.license_id
  if (!id) return
  const ok = await copyToClipboard(id)
  toast.show(ok ? 'License ID 已复制' : '复制失败，请手动选择复制')
}

onMounted(() => void load())

defineExpose({ saveLicenseCfg })
</script>

<template>
  <div class="cd-license">
    <div class="cd-license__intro">
      <h3 class="cd-settings-panel-title mb-1">授权与版本</h3>
      <p class="text-secondary small mb-0">查看当前授权状态、部署指纹与授权管理。</p>
    </div>

    <div class="cd-license__grid">
      <div class="cd-license__top-row">
        <section class="cd-license__block cd-license__block--edition" :class="editionHeroClass">
          <div class="cd-license__block-head cd-license__block-head--compact">
            <span class="cd-license__block-icon cd-license__block-icon--info">
              <IconShieldCheck :size="20" :stroke="1.75" />
            </span>
            <div class="cd-license__block-head-text">
              <h4 class="cd-license__block-title">版本信息</h4>
              <p class="cd-license__block-desc">当前运行版本与授权概况</p>
            </div>
            <div class="cd-license__head-actions ms-auto">
              <button
                type="button"
                class="btn btn-sm btn-ghost-primary cd-license__head-btn"
                @click="showUpdateLicense = true"
              >
                <IconCertificate :size="15" />
                更新授权
              </button>
              <button
                type="button"
                class="btn btn-sm btn-ghost-secondary cd-license__head-btn"
                :disabled="syncingLicense"
                @click="onSyncLicenseInfo"
              >
                <IconRefresh :size="15" />
                {{ syncingLicense ? '同步中…' : '同步授权' }}
              </button>
              <button
                type="button"
                class="btn btn-sm btn-ghost-secondary cd-license__head-btn"
                @click="openAdvancedCfg"
              >
                <IconAdjustments :size="15" />
                高级
              </button>
            </div>
          </div>

          <div class="cd-license__edition-hero">
            <div class="cd-license__edition-main">
              <span class="cd-license__edition-kicker">当前版本类型</span>
              <div class="cd-license__edition-row">
                <span class="cd-license__edition-name">{{ licenseStore.editionLabel }}</span>
              </div>
              <div class="cd-license__edition-version font-monospace">v{{ licenseStore.version }}</div>
              <div class="cd-license__edition-tags">
                <span class="badge" :class="licenseStatus.class">
                  <IconCircleCheck :size="12" class="me-1" />
                  {{ licenseStatus.text }}
                </span>
                <span class="badge bg-azure-lt text-azure">
                  {{ enabledFeatureCount }} 项功能已授权
                </span>
              </div>
            </div>
            <div class="cd-license__seat-card" :class="`cd-license__seat-card--${seatUsageTone}`">
              <div class="cd-license__seat-head">
                <IconUsers :size="16" :stroke="1.75" />
                <span>账号席位</span>
              </div>
              <div class="cd-license__seat-value">
                <span class="cd-license__seat-current">{{ licenseStore.userCount }}</span>
                <span class="cd-license__seat-sep">/</span>
                <span class="cd-license__seat-max">{{ licenseStore.maxUsersLabel }}</span>
              </div>
              <div v-if="seatUsagePercent != null" class="cd-license__seat-bar">
                <div class="cd-license__seat-bar-fill" :style="{ width: `${seatUsagePercent}%` }" />
              </div>
              <span v-else class="cd-license__seat-hint">无席位上限</span>
            </div>
          </div>

          <div class="cd-license__metrics">
            <div class="cd-license__metric">
              <span class="cd-license__metric-icon cd-license__metric-icon--customer">
                <IconBuilding :size="16" :stroke="1.75" />
              </span>
              <div class="cd-license__metric-body">
                <span class="cd-license__metric-label">授权客户</span>
                <span class="cd-license__metric-value">{{ licenseStore.license.customer_name || '—' }}</span>
              </div>
            </div>
            <div class="cd-license__metric">
              <span class="cd-license__metric-icon cd-license__metric-icon--expires">
                <IconCalendar :size="16" :stroke="1.75" />
              </span>
              <div class="cd-license__metric-body">
                <span class="cd-license__metric-label">有效期至</span>
                <span class="cd-license__metric-value">{{ formatExpiresAt(licenseStore.license.expires_at) }}</span>
              </div>
            </div>
          </div>

          <div v-if="licenseStore.license.license_id" class="cd-license__license-id">
            <span class="cd-license__license-id-label">License ID</span>
            <div class="cd-license__license-id-row">
              <code class="cd-license__license-id-code">{{ licenseStore.license.license_id }}</code>
              <button
                type="button"
                class="btn btn-sm btn-ghost-secondary cd-license__license-id-copy"
                title="复制 License ID"
                @click="copyLicenseId"
              >
                <IconCopy :size="15" />
              </button>
            </div>
          </div>

          <p v-if="licenseStore.license.reason" class="cd-license__alert text-danger small mb-0">
            {{ licenseStore.license.reason }}
          </p>
        </section>

        <section class="cd-license__block cd-license__block--fp">
          <div class="cd-license__block-head cd-license__block-head--compact">
            <span class="cd-license__block-icon cd-license__block-icon--fp">
              <IconScan :size="20" :stroke="1.75" />
            </span>
            <div>
              <h4 class="cd-license__block-title">部署指纹</h4>
              <p class="cd-license__block-desc">绑定授权时扫码或复制指纹</p>
            </div>
          </div>
          <div v-if="displayShortId" class="cd-license__fp-panel">
            <div class="cd-license__fp-qr-wrap">
              <canvas ref="fpQrCanvas" class="cd-license__fp-qr-canvas" />
              <span class="cd-license__fp-qr-hint text-secondary">扫码录入</span>
            </div>
            <div class="cd-license__fp-content">
              <span class="cd-license__fp-label">机器指纹</span>
              <code class="cd-license__fp-code">{{ displayShortId }}</code>
              <button type="button" class="btn btn-sm btn-primary cd-license__fp-copy" @click="copyFingerprint">
                <IconCopy :size="14" class="me-1" />
                复制指纹
              </button>
              <p v-if="machineFp?.hostname" class="cd-license__fp-meta text-secondary small mb-0">
                主机 {{ machineFp.hostname }}
                <template v-if="machineFp.platform"> · {{ machineFp.platform }}</template>
              </p>
            </div>
          </div>
          <div v-else class="cd-license__fp-empty text-secondary small">暂无部署指纹</div>
        </section>
      </div>

      <div class="cd-license__mid-row" :class="{ 'cd-license__mid-row--solo': !showTrialSection }">
        <section v-if="showTrialSection" class="cd-license__block cd-license__block--trial">
          <div class="cd-license__block-head">
            <span class="cd-license__block-icon cd-license__block-icon--trial">
              <IconGift :size="20" :stroke="1.75" />
            </span>
            <div class="cd-license__block-head-text">
              <h4 class="cd-license__block-title">试用授权</h4>
              <p class="cd-license__block-desc">社区版可自助申请 15 天试用，首次自动通过</p>
            </div>
            <span v-if="trialStatusBadge" class="badge ms-auto" :class="trialStatusBadge.class">
              {{ trialStatusBadge.text }}
            </span>
          </div>
          <div class="cd-license__block-body cd-license__trial-body">
            <p v-if="trialStatus?.message" class="small mb-3" :class="trialStatus.status === 'rejected' ? 'text-danger' : 'text-secondary'">
              {{ trialStatus.message }}
            </p>
            <template v-if="canApplyTrial">
              <div class="mb-2">
                <label class="form-label small">本机部署指纹</label>
                <input
                  :value="displayShortId"
                  class="form-control form-control-sm font-monospace"
                  readonly
                />
                <p class="text-secondary small mb-0 mt-1">申请将绑定此指纹，与上方「部署指纹」一致</p>
              </div>
              <div class="mb-2">
                <label class="form-label small">团队 / 企业名称</label>
                <input v-model="trialForm.team_name" class="form-control form-control-sm" placeholder="公司名称" />
              </div>
              <div class="mb-2">
                <label class="form-label small">联系邮箱</label>
                <input v-model="trialForm.contact" type="email" class="form-control form-control-sm" placeholder="name@company.com" />
              </div>
              <p class="text-secondary small mb-2">团队名称与联系邮箱至少填写一项</p>
              <div class="mb-3">
                <label class="form-label small">试用版本</label>
                <select v-model="trialForm.edition" class="form-select form-select-sm">
                  <option value="enterprise">旗舰版试用（15 天）</option>
                  <option value="team">团队版试用（15 天）</option>
                </select>
              </div>
              <button
                type="button"
                class="btn btn-sm btn-primary"
                :disabled="applyingTrial || !licenseCfg.license_server_url"
                @click="onApplyTrial"
              >
                {{ applyingTrial ? '提交中…' : '申请试用授权' }}
              </button>
            </template>
            <template v-else-if="trialStatus?.status === 'approved' && !hasActiveLicense">
              <div class="mb-2 small">
                <span class="text-secondary">试用版本：</span>
                <span class="badge bg-primary-lt text-primary">{{ trialEditionLabel(trialStatus.edition) }}</span>
                <span v-if="trialStatus.expires_at" class="text-secondary ms-2">有效期至 {{ trialStatus.expires_at }}</span>
              </div>
              <div class="mb-3">
                <label class="form-label small">License ID</label>
                <input
                  :value="trialStatus.license_id"
                  class="form-control form-control-sm font-monospace"
                  readonly
                />
              </div>
              <button
                type="button"
                class="btn btn-sm btn-primary"
                :disabled="activatingTrial"
                @click="onActivateTrial"
              >
                <IconCertificate :size="15" />
                {{ activatingTrial ? '激活中…' : '激活试用授权' }}
              </button>
            </template>
          </div>
        </section>

        <section class="cd-license__block cd-license__block--features">
          <div class="cd-license__block-head">
            <span class="cd-license__block-icon cd-license__block-icon--feat">
              <IconCertificate :size="20" :stroke="1.75" />
            </span>
            <div class="cd-license__block-head-text">
              <h4 class="cd-license__block-title">授权功能</h4>
              <p class="cd-license__block-desc">当前版本已开通的功能模块</p>
            </div>
            <span class="badge bg-secondary-lt text-secondary ms-auto">
              {{ enabledFeatureCount }}/{{ licensedFeatures.length }} 已授权
            </span>
          </div>
          <div class="cd-license__feature-list">
            <div
              v-for="f in licensedFeatures"
              :key="f.code"
              class="cd-license__feature-item"
            >
              <span>{{ f.label }}</span>
              <span class="badge" :class="f.enabled ? 'bg-green-lt text-green' : 'bg-secondary-lt text-secondary'">
                {{ f.enabled ? '已授权' : '未授权' }}
              </span>
            </div>
          </div>
        </section>
      </div>
    </div>

    <CdModal :show="showUpdateLicense" title="更新授权" size="md" @close="showUpdateLicense = false">
      <div class="modal-body">
        <p class="text-secondary small mb-3">导入离线授权文件，或使用 License ID 在线激活。</p>
        <div class="cd-license__update-section">
          <h6 class="cd-license__update-title">离线授权</h6>
          <button type="button" class="btn btn-outline-secondary" @click="fileInput?.click()">
            <IconUpload :size="15" /> 导入离线授权
          </button>
          <input ref="fileInput" type="file" accept=".lic,.json" class="d-none" @change="onImportFile" />
          <p class="form-hint mb-0">支持 .lic 或 .json 格式的授权文件</p>
        </div>
        <div class="cd-license__update-section">
          <h6 class="cd-license__update-title">在线激活</h6>
          <label class="form-label">License ID</label>
          <input
            v-model="activateId"
            class="form-control font-monospace mb-2"
            placeholder="UUID"
          />
          <button type="button" class="btn btn-primary" @click="onActivate">
            <IconCertificate :size="15" /> 在线激活
          </button>
          <p v-if="!licenseCfg.license_server_url.trim()" class="form-hint text-warning mb-0 mt-2">
            请先在「高级」中配置授权服务器地址
          </p>
        </div>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-link link-secondary" @click="showUpdateLicense = false">关闭</button>
      </div>
    </CdModal>

    <CdModal :show="showAdvancedCfg" title="授权高级设置" size="md" @close="showAdvancedCfg = false">
      <form @submit.prevent="saveAdvancedCfg">
        <div class="modal-body">
          <p class="text-secondary small mb-3">
            配置授权服务器地址，用于在线激活、试用申请与授权信息同步。
          </p>
          <div class="mb-3">
            <label class="form-label">授权服务器地址</label>
            <input
              v-model="advancedCfgDraft.license_server_url"
              type="text"
              class="form-control font-monospace"
              placeholder="https://license.nextfile.cn"
              autocomplete="off"
            />
            <div class="form-hint">默认：https://license.nextfile.cn</div>
          </div>
          <div v-if="hasActiveLicense" class="cd-license__advanced-actions">
            <button
              type="button"
              class="btn btn-outline-danger"
              :disabled="removing"
              @click="onRemoveLicense"
            >
              <IconTrash :size="15" />
              {{ removing ? '移除中…' : '移除授权' }}
            </button>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-link link-secondary" @click="showAdvancedCfg = false">取消</button>
          <button type="submit" class="btn btn-primary" :disabled="savingAdvancedCfg">
            {{ savingAdvancedCfg ? '保存中…' : '保存' }}
          </button>
        </div>
      </form>
    </CdModal>
  </div>
</template>

<style scoped>
.cd-license__intro {
  margin-bottom: 1.25rem;
}

.cd-license__grid {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.cd-license__top-row {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr);
  gap: 1rem;
  align-items: stretch;
}

.cd-license__mid-row {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr);
  gap: 1rem;
  align-items: stretch;
}

.cd-license__mid-row--solo .cd-license__block--features {
  grid-column: 2;
}

.cd-license__block {
  border: 1px solid var(--tblr-border-color);
  border-radius: var(--tblr-border-radius-lg, 0.5rem);
  background: var(--tblr-bg-surface, #fff);
  overflow: hidden;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.cd-license__block-head {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid var(--tblr-border-color-translucent, rgba(0, 0, 0, 0.06));
}

.cd-license__block-head--compact {
  padding: 0.85rem 1.1rem;
}

.cd-license__block-head-text {
  flex: 1;
  min-width: 0;
}

.cd-license__head-actions {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  flex-shrink: 0;
}

.cd-license__head-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.3rem 0.55rem;
}

.cd-license__update-section {
  padding: 0.85rem 0;
  border-bottom: 1px solid var(--tblr-border-color-translucent, rgba(0, 0, 0, 0.06));
}

.cd-license__update-section:last-child {
  border-bottom: 0;
  padding-bottom: 0;
}

.cd-license__update-section:first-child {
  padding-top: 0;
}

.cd-license__update-title {
  font-size: 0.8125rem;
  font-weight: 600;
  margin: 0 0 0.65rem;
}

.cd-license__update-section .btn {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.cd-license__advanced-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  padding-top: 0.85rem;
  border-top: 1px solid var(--tblr-border-color-translucent, rgba(0, 0, 0, 0.06));
}

.cd-license__advanced-actions .btn {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.cd-license__block-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 0.5rem;
  flex-shrink: 0;
}

.cd-license__block-icon--info {
  background: color-mix(in srgb, var(--tblr-primary) 14%, transparent);
  color: var(--tblr-primary);
}

.cd-license__block-icon--fp {
  background: color-mix(in srgb, var(--tblr-azure, #4299e1) 14%, transparent);
  color: var(--tblr-azure, #4299e1);
}

.cd-license__block-icon--mgmt {
  background: color-mix(in srgb, var(--tblr-orange, #f76707) 14%, transparent);
  color: var(--tblr-orange, #f76707);
}

.cd-license__block-icon--trial {
  background: color-mix(in srgb, var(--tblr-pink, #d6336c) 14%, transparent);
  color: var(--tblr-pink, #d6336c);
}

.cd-license__block-icon--feat {
  background: color-mix(in srgb, var(--tblr-purple, #ae3ec9) 14%, transparent);
  color: var(--tblr-purple, #ae3ec9);
}

.cd-license__block-title {
  font-size: 0.9375rem;
  font-weight: 600;
  margin: 0 0 0.125rem;
}

.cd-license__block-desc {
  font-size: 0.8125rem;
  color: var(--tblr-secondary);
  margin: 0;
}

.cd-license__block-body {
  padding: 1rem 1.25rem 1.25rem;
}

.cd-license__block--edition {
  overflow: hidden;
}

.cd-license__block--fp {
  background: linear-gradient(
    160deg,
    color-mix(in srgb, var(--tblr-azure, #4299e1) 7%, var(--tblr-bg-surface, #fff)) 0%,
    var(--tblr-bg-surface, #fff) 55%
  );
}

.cd-license__block--enterprise {
  background: linear-gradient(
    135deg,
    color-mix(in srgb, var(--tblr-primary) 10%, var(--tblr-bg-surface, #fff)) 0%,
    var(--tblr-bg-surface, #fff) 48%
  );
}

.cd-license__block--team {
  background: linear-gradient(
    135deg,
    color-mix(in srgb, var(--tblr-azure, #4299e1) 10%, var(--tblr-bg-surface, #fff)) 0%,
    var(--tblr-bg-surface, #fff) 48%
  );
}

.cd-license__block--community {
  background: linear-gradient(
    135deg,
    color-mix(in srgb, var(--tblr-secondary) 8%, var(--tblr-bg-surface, #fff)) 0%,
    var(--tblr-bg-surface, #fff) 48%
  );
}

.cd-license__edition-hero {
  display: flex;
  align-items: stretch;
  justify-content: space-between;
  gap: 1rem;
  padding: 1rem 1.1rem 0.85rem;
}

.cd-license__edition-main {
  flex: 1;
  min-width: 0;
}

.cd-license__edition-kicker {
  display: block;
  font-size: 0.6875rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--tblr-secondary);
  margin-bottom: 0.35rem;
}

.cd-license__edition-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem 0.75rem;
}

.cd-license__edition-name {
  font-size: clamp(1.35rem, 2.5vw, 1.75rem);
  font-weight: 700;
  line-height: 1.15;
  letter-spacing: -0.02em;
}

.cd-license__edition-version {
  margin-top: 0.35rem;
  font-size: 0.9375rem;
  color: var(--tblr-secondary);
}

.cd-license__edition-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.85rem;
}

.cd-license__edition-tags .badge {
  display: inline-flex;
  align-items: center;
  font-weight: 500;
  padding: 0.35em 0.65em;
}

.cd-license__seat-card {
  flex-shrink: 0;
  width: min(100%, 10rem);
  padding: 0.75rem 0.85rem;
  border-radius: var(--tblr-border-radius-lg, 0.5rem);
  border: 1px solid var(--tblr-border-color-translucent, rgba(0, 0, 0, 0.08));
  background: color-mix(in srgb, var(--tblr-bg-surface, #fff) 82%, transparent);
  backdrop-filter: blur(6px);
}

.cd-license__seat-card--ok {
  border-color: color-mix(in srgb, var(--tblr-green, #2fb344) 28%, transparent);
}

.cd-license__seat-card--warn {
  border-color: color-mix(in srgb, var(--tblr-yellow, #f59f00) 35%, transparent);
}

.cd-license__seat-card--danger {
  border-color: color-mix(in srgb, var(--tblr-danger) 35%, transparent);
}

.cd-license__seat-head {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.75rem;
  color: var(--tblr-secondary);
  margin-bottom: 0.35rem;
}

.cd-license__seat-value {
  display: flex;
  align-items: baseline;
  gap: 0.2rem;
  line-height: 1.1;
}

.cd-license__seat-current {
  font-size: 1.5rem;
  font-weight: 700;
}

.cd-license__seat-sep,
.cd-license__seat-max {
  font-size: 1rem;
  color: var(--tblr-secondary);
}

.cd-license__seat-bar {
  margin-top: 0.65rem;
  height: 0.35rem;
  border-radius: 999px;
  background: var(--tblr-border-color-translucent, rgba(0, 0, 0, 0.08));
  overflow: hidden;
}

.cd-license__seat-bar-fill {
  height: 100%;
  border-radius: inherit;
  background: var(--tblr-green, #2fb344);
  transition: width 0.25s ease;
}

.cd-license__seat-card--warn .cd-license__seat-bar-fill {
  background: var(--tblr-yellow, #f59f00);
}

.cd-license__seat-card--danger .cd-license__seat-bar-fill {
  background: var(--tblr-danger);
}

.cd-license__seat-hint {
  display: block;
  margin-top: 0.5rem;
  font-size: 0.6875rem;
  color: var(--tblr-secondary);
}

.cd-license__metrics {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.65rem;
  padding: 0 1.1rem 0.85rem;
}

.cd-license__metric {
  display: flex;
  align-items: flex-start;
  gap: 0.65rem;
  padding: 0.85rem 0.9rem;
  border-radius: var(--tblr-border-radius);
  border: 1px solid var(--tblr-border-color-translucent, rgba(0, 0, 0, 0.06));
  background: var(--tblr-bg-surface, #fff);
  min-width: 0;
}

.cd-license__metric-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border-radius: 0.45rem;
  flex-shrink: 0;
}

.cd-license__metric-icon--customer {
  background: color-mix(in srgb, var(--tblr-orange, #f76707) 14%, transparent);
  color: var(--tblr-orange, #f76707);
}

.cd-license__metric-icon--expires {
  background: color-mix(in srgb, var(--tblr-purple, #ae3ec9) 14%, transparent);
  color: var(--tblr-purple, #ae3ec9);
}

.cd-license__metric-body {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}

.cd-license__metric-label {
  font-size: 0.6875rem;
  color: var(--tblr-secondary);
}

.cd-license__metric-value {
  font-size: 0.875rem;
  font-weight: 600;
  word-break: break-word;
}

.cd-license__license-id {
  margin: 0 1.1rem 1rem;
  margin-top: auto;
  padding: 0.65rem 0.75rem;
  border-radius: var(--tblr-border-radius);
  border: 1px dashed var(--tblr-border-color);
  background: var(--tblr-bg-surface-secondary, #f8fafc);
}

.cd-license__license-id-label {
  display: block;
  font-size: 0.6875rem;
  color: var(--tblr-secondary);
  margin-bottom: 0.35rem;
}

.cd-license__license-id-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.cd-license__license-id-code {
  flex: 1;
  min-width: 0;
  font-size: 0.8125rem;
  word-break: break-all;
  color: var(--tblr-body-color);
  background: transparent;
}

.cd-license__license-id-copy {
  flex-shrink: 0;
  padding: 0.25rem 0.45rem;
}

.cd-license__alert {
  margin: 0 1.1rem 1rem;
  margin-top: auto;
  padding: 0.5rem 0.75rem;
  border-radius: var(--tblr-border-radius);
  background: color-mix(in srgb, var(--tblr-danger) 8%, transparent);
}

.cd-license__fp-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 1rem;
  padding: 1rem 1.1rem 1.25rem;
  text-align: center;
}

.cd-license__fp-qr-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.35rem;
}

.cd-license__fp-content {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 0.5rem;
  width: 100%;
}

.cd-license__fp-label {
  font-size: 0.6875rem;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--tblr-secondary);
  text-align: left;
}

.cd-license__fp-code {
  width: 100%;
  font-size: clamp(1rem, 2vw, 1.35rem);
  font-weight: 600;
  letter-spacing: 0.05em;
  padding: 0.55rem 0.7rem;
  border-radius: var(--tblr-border-radius);
  background: color-mix(in srgb, var(--tblr-bg-surface, #fff) 70%, transparent);
  border: 1px solid var(--tblr-border-color-translucent, rgba(0, 0, 0, 0.08));
  word-break: break-all;
  text-align: center;
}

.cd-license__fp-copy {
  align-self: center;
}

.cd-license__fp-meta {
  text-align: center;
  margin-top: 0.15rem;
}

.cd-license__fp-qr-canvas {
  display: block;
  border-radius: var(--tblr-border-radius);
  border: 1px solid var(--tblr-border-color-translucent, rgba(0, 0, 0, 0.06));
  background: #fff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.cd-license__fp-qr-hint {
  font-size: 0.6875rem;
}

.cd-license__fp-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem 1.1rem;
}

.cd-license__block--features {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.cd-license__trial-body {
  flex: 1;
}

.cd-license__btn-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.cd-license__feature-list {
  flex: 1;
  height: 18rem;
  max-height: 18rem;
  overflow-y: auto;
  border-top: 1px solid var(--tblr-border-color-translucent, rgba(0, 0, 0, 0.06));
  padding: 0.5rem 1.1rem 0.75rem;
  overscroll-behavior: contain;
}

.cd-license__feature-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.5rem 0;
  font-size: 0.8125rem;
  border-bottom: 1px solid var(--tblr-border-color-translucent, rgba(0, 0, 0, 0.04));
}

.cd-license__feature-item:last-child {
  border-bottom: 0;
}

@media (max-width: 992px) {
  .cd-license__top-row,
  .cd-license__mid-row {
    grid-template-columns: 1fr;
  }

  .cd-license__mid-row--solo .cd-license__block--features {
    grid-column: auto;
  }
}

@media (max-width: 768px) {
  .cd-license__edition-hero {
    flex-direction: column;
  }

  .cd-license__seat-card {
    width: 100%;
  }

  .cd-license__metrics {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 760px) {
  .cd-license__block-head { flex-wrap: wrap; }
  .cd-license__head-actions { flex-wrap: wrap; max-width: 100%; margin-left: 0 !important; }
}
</style>
