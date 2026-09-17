<script setup lang="ts">
import { ref } from 'vue'
import { ApiError, request } from '@/api/client'
import { saveSettings } from '@/api/settings'
import { useToast } from '@/composables/useToast'

interface EmailTemplates {
  reset_password: string
  admin_reset_password: string
  test: string
}

interface EmailSettings {
  enabled: boolean
  smtp_host: string
  smtp_port: number
  smtp_user: string
  smtp_pass: string
  from: string
  use_tls: boolean
  templates: EmailTemplates
}

const toast = useToast()
const form = defineModel<EmailSettings>({ required: true })
const saving = ref(false)
const testing = ref(false)
const testTo = ref('')

async function save() {
  saving.value = true
  try {
    form.value = await saveSettings<EmailSettings>('email', form.value)
    toast.show('邮件配置已保存')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}

async function testSend() {
  if (!testTo.value.trim()) {
    toast.show('请填写测试收件邮箱')
    return
  }
  testing.value = true
  try {
    await request<{ message?: string }>('/api/admin/settings/email/test', {
      method: 'POST',
      body: JSON.stringify({ to: testTo.value.trim() }),
    })
    toast.show('测试邮件已发送')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '发送失败')
  } finally {
    testing.value = false
  }
}
</script>

<template>
  <div class="cd-settings-panel">
    <h3 class="cd-settings-panel-title">邮件配置</h3>
    <p class="text-secondary small mb-4">
      SMTP 配置、测试发送与邮件模板。模板变量：
      <code v-pre>{{username}}</code>、<code v-pre>{{password}}</code>、<code v-pre>{{link}}</code>、<code v-pre>{{site_name}}</code>
    </p>

    <label class="form-check form-switch mb-4">
      <input v-model="form.enabled" class="form-check-input" type="checkbox" />
      <span class="form-check-label">启用邮件发送</span>
    </label>

    <div class="cd-settings-section">
      <h4>SMTP 服务器</h4>
      <div class="row g-3 mb-3">
        <div class="col-md-6">
          <label class="form-label">SMTP 主机</label>
          <input v-model="form.smtp_host" class="form-control" placeholder="smtp.example.com" />
        </div>
        <div class="col-md-3">
          <label class="form-label">端口</label>
          <input v-model.number="form.smtp_port" type="number" class="form-control" placeholder="587" />
        </div>
        <div class="col-md-3 d-flex align-items-end">
          <label class="form-check">
            <input v-model="form.use_tls" type="checkbox" class="form-check-input" />
            <span class="form-check-label">启用 TLS</span>
          </label>
        </div>
        <div class="col-md-4">
          <label class="form-label">用户名</label>
          <input v-model="form.smtp_user" class="form-control" autocomplete="off" />
        </div>
        <div class="col-md-4">
          <label class="form-label">密码</label>
          <input v-model="form.smtp_pass" type="password" class="form-control" autocomplete="new-password" />
        </div>
        <div class="col-md-4">
          <label class="form-label">发件人</label>
          <input v-model="form.from" class="form-control" placeholder="noreply@example.com" />
        </div>
      </div>
    </div>

    <div class="cd-settings-section">
      <h4>测试发送</h4>
      <div class="input-group mb-3" style="max-width: 28rem">
        <input v-model="testTo" type="email" class="form-control" placeholder="收件邮箱" />
        <button type="button" class="btn btn-outline-primary" :disabled="testing" @click="testSend">
          {{ testing ? '发送中…' : '发送测试' }}
        </button>
      </div>
      <p class="text-secondary small">保存配置后再点击测试，将使用当前表单中的 SMTP 设置。</p>
    </div>

    <div class="cd-settings-section">
      <h4>邮件模板</h4>
      <div class="mb-3">
        <label class="form-label">重置密码（自助找回）</label>
        <textarea v-model="form.templates.reset_password" class="form-control font-monospace" rows="3" />
      </div>
      <div class="mb-3">
        <label class="form-label">管理员重置密码</label>
        <textarea v-model="form.templates.admin_reset_password" class="form-control font-monospace" rows="4" />
      </div>
      <div class="mb-3">
        <label class="form-label">测试邮件</label>
        <textarea v-model="form.templates.test" class="form-control font-monospace" rows="2" />
      </div>
    </div>

    <div class="cd-settings-actions">
      <button type="button" class="btn btn-primary" :disabled="saving" @click="save">
        {{ saving ? '保存中…' : '保存' }}
      </button>
    </div>
  </div>
</template>
