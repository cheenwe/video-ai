<script setup lang="ts">
import { ref } from 'vue'
import { ApiError } from '@/api/client'
import { saveSettings } from '@/api/settings'
import { useToast } from '@/composables/useToast'

interface OAuthProvider {
  id: string
  name: string
  type: string // github, microsoft, google, custom
  client_id: string
  client_secret: string
  auth_url?: string
  token_url?: string
  user_info_url?: string
  scopes?: string
  icon_url?: string
  enabled: boolean
}

interface OAuthSettings {
  enabled: boolean
  providers: OAuthProvider[]
}

const toast = useToast()
const form = defineModel<OAuthSettings>({ required: true })
const saving = ref(false)

const PRESETS: Array<{ id: string; name: string; type: string; scopes: string }> = [
  { id: 'github', name: 'GitHub', type: 'github', scopes: 'read:user user:email' },
  { id: 'microsoft', name: 'Microsoft', type: 'microsoft', scopes: 'openid profile email' },
  { id: 'google', name: 'Google', type: 'google', scopes: 'openid profile email' },
]

function newProvider(): OAuthProvider {
  return {
    id: `custom_${Date.now()}`,
    name: '自定义',
    type: 'custom',
    client_id: '',
    client_secret: '',
    scopes: '',
    enabled: true,
  }
}

function addProvider() {
  form.value.providers = [...(form.value.providers || []), newProvider()]
}

function addPreset(preset: { id: string; name: string; type: string; scopes: string }) {
  if ((form.value.providers || []).some((p) => p.id === preset.id)) {
    toast.show(`已存在「${preset.name}」`)
    return
  }
  form.value.providers = [
    ...(form.value.providers || []),
    {
      id: preset.id,
      name: preset.name,
      type: preset.type,
      client_id: '',
      client_secret: '',
      scopes: preset.scopes,
      enabled: false,
    },
  ]
}

function removeProvider(i: number) {
  form.value.providers.splice(i, 1)
}

function readIcon(e: Event, p: OAuthProvider) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  if (file.size > 128 * 1024) {
    toast.show('图标请小于 128KB')
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    p.icon_url = String(reader.result || '')
  }
  reader.readAsDataURL(file)
  ;(e.target as HTMLInputElement).value = ''
}

function callbackHint(id: string) {
  const origin = typeof window !== 'undefined' ? window.location.origin : 'https://your-domain'
  return `${origin}/api/auth/oauth/${encodeURIComponent(id)}/callback`
}

async function save() {
  saving.value = true
  try {
    form.value = await saveSettings<OAuthSettings>('oauth', form.value)
    toast.show('OAuth 配置已保存')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="cd-settings-panel">
    <h3 class="cd-settings-panel-title">OAuth 登录</h3>
    <p class="text-secondary small mb-4">
      仅用于<strong>用户登录</strong>。配置各 OAuth 提供商的 Client ID / Secret 后，登录页将显示对应入口。
    </p>

    <label class="form-check form-switch mb-4">
      <input v-model="form.enabled" class="form-check-input" type="checkbox" />
      <span class="form-check-label">启用 OAuth 登录</span>
    </label>

    <div class="d-flex flex-wrap gap-2 mb-4">
      <button
        v-for="preset in PRESETS"
        :key="preset.id"
        type="button"
        class="btn btn-sm btn-outline-secondary"
        @click="addPreset(preset)"
      >
        + {{ preset.name }}
      </button>
      <button type="button" class="btn btn-sm btn-outline-primary" @click="addProvider">
        + 自定义
      </button>
    </div>

    <div v-for="(p, i) in form.providers" :key="p.id" class="card mb-3">
      <div class="card-body">
        <div class="d-flex justify-content-between align-items-start mb-3">
          <strong>{{ p.name || `提供商 ${i + 1}` }}</strong>
          <button type="button" class="btn btn-sm btn-outline-danger" @click="removeProvider(i)">
            删除
          </button>
        </div>
        <div class="row g-3">
          <div class="col-md-4">
            <label class="form-label">标识 ID</label>
            <input v-model="p.id" class="form-control form-control-sm" />
            <div class="form-hint">回调：{{ callbackHint(p.id) }}</div>
          </div>
          <div class="col-md-4">
            <label class="form-label">显示名称</label>
            <input v-model="p.name" class="form-control form-control-sm" />
          </div>
          <div class="col-md-4">
            <label class="form-label">类型</label>
            <select v-model="p.type" class="form-select form-select-sm">
              <option value="github">GitHub</option>
              <option value="microsoft">Microsoft（登录）</option>
              <option value="google">Google（登录）</option>
              <option value="custom">自定义</option>
            </select>
          </div>
          <div class="col-md-6">
            <label class="form-label">Client ID</label>
            <input v-model="p.client_id" class="form-control form-control-sm" autocomplete="off" />
          </div>
          <div class="col-md-6">
            <label class="form-label">Client Secret</label>
            <input v-model="p.client_secret" type="password" class="form-control form-control-sm" autocomplete="new-password" />
          </div>
          <template v-if="p.type === 'custom' || p.type === 'microsoft' || p.type === 'google'">
            <div class="col-md-4">
              <label class="form-label">授权 URL</label>
              <input v-model="p.auth_url" class="form-control form-control-sm" />
            </div>
            <div class="col-md-4">
              <label class="form-label">Token URL</label>
              <input v-model="p.token_url" class="form-control form-control-sm" />
            </div>
            <div class="col-md-4">
              <label class="form-label">用户信息 URL</label>
              <input v-model="p.user_info_url" class="form-control form-control-sm" />
            </div>
          </template>
          <div class="col-md-6">
            <label class="form-label">Scopes</label>
            <input v-model="p.scopes" class="form-control form-control-sm" placeholder="read:user user:email" />
          </div>
          <div class="col-md-6">
            <label class="form-label">图标</label>
            <div class="d-flex align-items-center gap-2">
              <img v-if="p.icon_url" :src="p.icon_url" alt="" style="width: 1.5rem; height: 1.5rem; object-fit: contain" />
              <input type="file" accept="image/*" class="form-control form-control-sm" @change="readIcon($event, p)" />
            </div>
          </div>
          <div class="col-12">
            <label class="form-check">
              <input v-model="p.enabled" class="form-check-input" type="checkbox" />
              <span class="form-check-label">启用此提供商</span>
            </label>
          </div>
        </div>
      </div>
    </div>

    <p v-if="!form.providers || !form.providers.length" class="text-secondary small mb-4">
      暂无 OAuth 提供商，点上方按钮添加。
    </p>

    <div class="cd-settings-actions">
      <button type="button" class="btn btn-primary" :disabled="saving" @click="save">
        {{ saving ? '保存中…' : '保存' }}
      </button>
    </div>
  </div>
</template>
