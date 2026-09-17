<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ApiError } from '@/api/client'
import { saveSettings, captchaPreview } from '@/api/settings'
import type { CaptchaConfig } from '@/api/settings'
import { totpStatus, totpSetup, totpConfirm, totpDisable } from '@/api/auth'
import { useLicenseStore } from '@/stores/license'
import { useToast } from '@/composables/useToast'

interface SecuritySettings {
  captcha_enabled: boolean
  captcha: CaptchaConfig | null
  login_fail_window_sec: number
  login_fail_max: number
  login_ban_sec: number
  password_min_len: number
  password_require_upper: boolean
  password_require_lower: boolean
  password_require_digit: boolean
  password_require_special: boolean
  admin_2fa_enabled: boolean
}

const toast = useToast()
const licenseStore = useLicenseStore()
const form = defineModel<SecuritySettings>({ required: true })
const saving = ref(false)
const totpEnabled = ref(false)
const totpRequired = ref(false)
const totpQr = ref('')
const totpCode = ref('')
const totpLoading = ref(false)

const previewImg = ref('')
const previewLoading = ref(false)
let previewTimer: ReturnType<typeof setTimeout> | undefined

const canEditTOTP = computed(() => licenseStore.hasFeature('admin_totp'))

function defaultCaptcha(): CaptchaConfig {
  return { type: 'digit', length: 4, noise_level: 'low', width: 120, height: 44, expiry_seconds: 180, case_sensitive: false, obfuscate_image: true }
}

async function refreshPreview() {
  if (!form.value.captcha) return
  previewLoading.value = true
  try {
    const res = await captchaPreview(form.value.captcha)
    previewImg.value = `data:image/png;base64,${res.image_base64}`
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '预览失败')
  } finally {
    previewLoading.value = false
  }
}

function schedulePreview() {
  if (!form.value.captcha_enabled) return
  clearTimeout(previewTimer)
  previewTimer = setTimeout(refreshPreview, 350)
}

watch(
  () => form.value.captcha_enabled,
  (enabled) => {
    if (enabled) refreshPreview()
  },
)
watch(
  () => {
    const c = form.value.captcha
    return c ? [c.type, c.length, c.noise_level, c.width, c.height].join('|') : ''
  },
  schedulePreview,
)

async function loadTOTP() {
  try {
    const s = await totpStatus()
    totpEnabled.value = s.enabled
    totpRequired.value = s.enforced
  } catch {
    /* ignore */
  }
}

async function save() {
  saving.value = true
  try {
    form.value = await saveSettings<SecuritySettings>('security', form.value)
    await loadTOTP()
    toast.show('安全配置已保存')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}

async function setupTOTP() {
  totpLoading.value = true
  try {
    const res = await totpSetup()
    totpQr.value = res.qr_url
    toast.show('请扫码后输入验证码确认')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '设置失败')
  } finally {
    totpLoading.value = false
  }
}

async function confirmTOTP() {
  totpLoading.value = true
  try {
    await totpConfirm(totpCode.value.trim())
    totpQr.value = ''
    totpCode.value = ''
    totpEnabled.value = true
    toast.show('双因素认证已启用')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '确认失败')
  } finally {
    totpLoading.value = false
  }
}

async function disableTOTP() {
  totpLoading.value = true
  try {
    await totpDisable(totpCode.value.trim())
    totpCode.value = ''
    totpEnabled.value = false
    toast.show('双因素认证已关闭')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '关闭失败')
  } finally {
    totpLoading.value = false
  }
}

onMounted(() => {
  if (!form.value.captcha) form.value.captcha = defaultCaptcha()
  loadTOTP()
  if (form.value.captcha_enabled) refreshPreview()
})
</script>

<template>
  <div class="cd-settings-panel">
    <h3 class="cd-settings-panel-title">安全配置</h3>
    <p class="text-secondary small mb-4">登录验证码、失败封禁、密码规则与管理员双因素认证。</p>

    <div class="cd-settings-section">
      <h4>登录保护</h4>
      <label class="form-check form-switch mb-3">
        <input v-model="form.captcha_enabled" class="form-check-input" type="checkbox" />
        <span class="form-check-label">登录时启用验证码</span>
      </label>
      <div v-if="form.captcha_enabled && form.captcha" class="cd-captcha-config mb-4">
        <div class="row g-3">
          <div class="col-md-3">
            <label class="form-label">验证码类型</label>
            <select v-model="form.captcha.type" class="form-select">
              <option value="digit">纯数字</option>
              <option value="string">字母 + 数字</option>
              <option value="math">算术运算</option>
            </select>
          </div>
          <div v-if="form.captcha.type !== 'math'" class="col-md-3">
            <label class="form-label">字符数量</label>
            <input v-model.number="form.captcha.length" type="number" min="2" max="8" class="form-control" />
          </div>
          <div class="col-md-3">
            <label class="form-label">干扰强度</label>
            <select v-model="form.captcha.noise_level" class="form-select">
              <option value="low">低（清晰易识别）</option>
              <option value="medium">中</option>
              <option value="high">高</option>
            </select>
          </div>
          <div class="col-md-3">
            <label class="form-label">尺寸（宽 × 高）</label>
            <div class="d-flex align-items-center gap-2">
              <input v-model.number="form.captcha.width" type="number" min="80" max="400" class="form-control" />
              <span class="text-secondary">×</span>
              <input v-model.number="form.captcha.height" type="number" min="30" max="200" class="form-control" />
            </div>
          </div>
        </div>
        <div class="row g-3 mt-1">
          <div class="col-md-3">
            <label class="form-label">有效期（秒）</label>
            <input v-model.number="form.captcha.expiry_seconds" type="number" min="30" max="1800" class="form-control" />
          </div>
          <div class="col-md-4 d-flex align-items-end">
            <label class="form-check mb-2">
              <input v-model="form.captcha.obfuscate_image" type="checkbox" class="form-check-input" />
              <span class="form-check-label">下发前混淆图片</span>
            </label>
          </div>
          <div v-if="form.captcha.type === 'string'" class="col-md-4 d-flex align-items-end">
            <label class="form-check mb-2">
              <input v-model="form.captcha.case_sensitive" type="checkbox" class="form-check-input" />
              <span class="form-check-label">校验区分大小写</span>
            </label>
          </div>
        </div>
        <p class="text-secondary small mt-2 mb-0">
          验证码答案仅保存在服务端、从不下发；图片为一次性使用，校验后立即失效并受有效期限制。
          干扰强度越高越能对抗自动识别（OCR），但会降低可读性，请按需权衡。
          开启「混淆图片」后，图片字节下发前会被异或打乱、由前端还原再显示，可提高脚本直接抓图 OCR 的门槛（非加密，仅增加难度）。
        </p>
        <div class="d-flex align-items-center gap-3 mt-3">
          <div class="cd-captcha-preview">
            <img v-if="previewImg" :src="previewImg" alt="验证码预览" />
            <span v-else class="text-secondary small">暂无预览</span>
          </div>
          <button type="button" class="btn btn-sm btn-outline-primary" :disabled="previewLoading" @click="refreshPreview">
            {{ previewLoading ? '生成中…' : '刷新预览' }}
          </button>
        </div>
      </div>
      <p class="text-secondary small mb-2">
        未全局开启时，同一账号连续密码错误达到 3 次后也会强制要求验证码，登录成功后清零。
      </p>
      <div class="row g-3">
        <div class="col-md-4">
          <label class="form-label">失败统计窗口（秒）</label>
          <input v-model.number="form.login_fail_window_sec" type="number" min="30" class="form-control" />
        </div>
        <div class="col-md-4">
          <label class="form-label">窗口内最大失败次数</label>
          <input v-model.number="form.login_fail_max" type="number" min="1" class="form-control" />
        </div>
        <div class="col-md-4">
          <label class="form-label">封禁时长（秒）</label>
          <input v-model.number="form.login_ban_sec" type="number" min="60" class="form-control" />
        </div>
      </div>
    </div>

    <div class="cd-settings-section">
      <h4>密码规则</h4>
      <div class="row g-3">
        <div class="col-md-3">
          <label class="form-label">最小长度</label>
          <input v-model.number="form.password_min_len" type="number" min="6" max="128" class="form-control" />
        </div>
        <div class="col-md-9 d-flex flex-wrap gap-3 align-items-end">
          <label class="form-check"><input v-model="form.password_require_upper" type="checkbox" class="form-check-input" /><span class="form-check-label">大写字母</span></label>
          <label class="form-check"><input v-model="form.password_require_lower" type="checkbox" class="form-check-input" /><span class="form-check-label">小写字母</span></label>
          <label class="form-check"><input v-model="form.password_require_digit" type="checkbox" class="form-check-input" /><span class="form-check-label">数字</span></label>
          <label class="form-check"><input v-model="form.password_require_special" type="checkbox" class="form-check-input" /><span class="form-check-label">特殊字符</span></label>
        </div>
      </div>
    </div>

    <div class="cd-settings-section">
      <h4>管理员双因素认证 (TOTP)</h4>
      <p v-if="!canEditTOTP" class="text-secondary small mb-2">管理员双因素认证需旗舰版授权（admin_totp）。已绑定账号登录时仍可能要求验证码。</p>
      <label class="form-check form-switch mb-3">
        <input v-model="form.admin_2fa_enabled" class="form-check-input" type="checkbox" :disabled="!canEditTOTP" />
        <span class="form-check-label">要求管理员登录时验证 TOTP</span>
      </label>
      <p class="text-secondary small">
        当前账号 2FA：{{ totpEnabled ? '已启用' : '未启用' }}
        <span v-if="totpRequired && !totpEnabled" class="text-warning">（启用全局 2FA 后需完成绑定）</span>
        <span v-else-if="totpRequired && totpEnabled" class="text-warning">（已被全局策略强制，无法关闭）</span>
      </p>
      <div class="d-flex flex-wrap gap-2 mb-3">
        <button v-if="!totpEnabled" type="button" class="btn btn-sm btn-outline-primary" :disabled="totpLoading || !canEditTOTP" @click="setupTOTP">
          生成绑定二维码
        </button>
      </div>
      <div v-if="totpQr" class="mb-3">
        <img :src="totpQr" alt="TOTP QR" class="cd-totp-qr" />
      </div>
      <div class="row g-2 align-items-end">
        <div class="col-md-4">
          <label class="form-label">验证码</label>
          <input v-model="totpCode" class="form-control" placeholder="6 位数字" maxlength="8" :disabled="!canEditTOTP" />
        </div>
        <div class="col-auto d-flex gap-2">
          <button v-if="!totpEnabled" type="button" class="btn btn-sm btn-primary" :disabled="totpLoading || !canEditTOTP" @click="confirmTOTP">确认启用</button>
          <button v-else-if="!totpRequired" type="button" class="btn btn-sm btn-outline-danger" :disabled="totpLoading || !canEditTOTP" @click="disableTOTP">关闭 2FA</button>
        </div>
      </div>
    </div>

    <div class="cd-settings-actions">
      <button type="button" class="btn btn-primary" :disabled="saving" @click="save">
        {{ saving ? '保存中…' : '保存' }}
      </button>
    </div>
  </div>
</template>
