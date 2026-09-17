<script setup lang="ts">
import { ref } from 'vue'
import { ApiError } from '@/api/client'
import { saveSettings } from '@/api/settings'
import { useToast } from '@/composables/useToast'

interface GatewaySettings {
  personal_tokens_enabled: boolean
}

const toast = useToast()
const form = defineModel<GatewaySettings>({ required: true })
const saving = ref(false)

async function save() {
  saving.value = true
  try {
    form.value = await saveSettings<GatewaySettings>('gateway', form.value)
    toast.show('网关配置已保存')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="cd-settings-panel">
    <h3 class="cd-settings-panel-title">对外网关</h3>
    <p class="text-secondary small mb-4">
      应用本身提供 REST API（JWT / 个人访问令牌 + PathACL）。此处可控制个人访问令牌入口。
    </p>

    <div class="cd-settings-section">
      <h4>个人访问令牌</h4>
      <label class="form-check form-switch mb-3">
        <input v-model="form.personal_tokens_enabled" class="form-check-input" type="checkbox" />
        <span class="form-check-label">启用个人访问令牌（Profile / REST）</span>
      </label>
      <p class="text-secondary small mb-0">
        开启后，用户可在个人资料中生成 <code>nf_…</code> 前缀的个人访问令牌，作为 REST API 的 Bearer 凭据使用。
        可在 Header <code>X-API-Key</code> 或 <code>Authorization: Bearer</code> 中携带。
      </p>
    </div>

    <div class="alert alert-secondary small mb-0">
      <strong>guest 账号：</strong>系统种子用户 <code>guest</code>（默认密码 <code>guest</code>），禁止 Web 登录。
      可由管理员在用户管理中代发个人访问令牌用于程序化访问。
    </div>

    <div class="cd-settings-actions">
      <button type="button" class="btn btn-primary" :disabled="saving" @click="save">
        {{ saving ? '保存中…' : '保存' }}
      </button>
    </div>
  </div>
</template>
