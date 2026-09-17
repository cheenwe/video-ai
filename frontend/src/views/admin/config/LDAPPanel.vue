<script setup lang="ts">
import { ref } from 'vue'
import { ApiError, request } from '@/api/client'
import { saveSettings } from '@/api/settings'
import { useToast } from '@/composables/useToast'

interface LDAPSettings {
  enabled: boolean
  host: string
  port: number
  use_tls: boolean
  bind_dn: string
  bind_pass: string
  base_dn: string
  user_filter: string
  username_attr: string
  email_attr: string
  sync_enabled: boolean
  dept_base_dn: string
  dept_filter: string
  dept_name_attr: string
  user_sync_filter: string
  test_username: string
}

const toast = useToast()
const form = defineModel<LDAPSettings>({ required: true })
const saving = ref(false)
const testing = ref(false)

async function save() {
  saving.value = true
  try {
    form.value = await saveSettings<LDAPSettings>('ldap', form.value)
    toast.show('LDAP 配置已保存')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}

async function testConn() {
  testing.value = true
  try {
    form.value = await saveSettings<LDAPSettings>('ldap', form.value)
    const res = await request<{ message?: string }>('/api/admin/settings/ldap/test', {
      method: 'POST',
    })
    toast.show(res?.message || 'LDAP 连接成功')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '连接失败')
  } finally {
    testing.value = false
  }
}

/** Windows AD 域控常用默认（不覆盖主机 / Bind / Base DN / 测试用户名） */
function applyWindowsADDefaults() {
  form.value = {
    ...form.value,
    port: 389,
    use_tls: false,
    user_filter: '(sAMAccountName=%s)',
    username_attr: 'sAMAccountName',
    email_attr: 'mail',
    sync_enabled: true,
    dept_filter: '(objectClass=organizationalUnit)',
    dept_name_attr: 'name',
    user_sync_filter:
      '(&(objectCategory=person)(objectClass=user)(!(userAccountControl:1.2.840.113556.1.4.803:=2)))',
  }
  toast.show('已填入 Windows 域控默认参数，请核对主机与 Base DN 后保存')
}
</script>

<template>
  <div class="cd-settings-panel">
    <div class="d-flex flex-wrap align-items-start justify-content-between gap-2 mb-2">
      <h3 class="cd-settings-panel-title mb-0">LDAP / 域集成</h3>
      <button type="button" class="btn btn-sm btn-outline-secondary" @click="applyWindowsADDefaults">
        Windows 域控默认
      </button>
    </div>
    <p class="text-secondary small mb-4">
      启用后，统一登录框按账号来源分流：LDAP 账号走目录认证，本地账号走本地密码。
      可点右上角「Windows 域控默认」一键填入 AD 常用过滤器与属性（不覆盖主机、Bind、Base DN）。
    </p>

    <label class="form-check form-switch mb-4">
      <input v-model="form.enabled" class="form-check-input" type="checkbox" />
      <span class="form-check-label">启用 LDAP 登录</span>
    </label>

    <div class="cd-settings-section">
      <h4>连接参数</h4>
      <div class="row g-3">
        <div class="col-md-6">
          <label class="form-label">主机</label>
          <input v-model="form.host" class="form-control" placeholder="dc.example.com" />
        </div>
        <div class="col-md-3">
          <label class="form-label">端口</label>
          <input v-model.number="form.port" type="number" class="form-control" placeholder="389" />
        </div>
        <div class="col-md-3 d-flex align-items-end">
          <label class="form-check">
            <input v-model="form.use_tls" type="checkbox" class="form-check-input" />
            <span class="form-check-label">LDAPS</span>
          </label>
        </div>
        <div class="col-md-6">
          <label class="form-label">Bind DN</label>
          <input v-model="form.bind_dn" class="form-control" placeholder="CN=Administrator,CN=Users,DC=example,DC=com" />
        </div>
        <div class="col-md-6">
          <label class="form-label">Bind 密码</label>
          <input v-model="form.bind_pass" type="password" class="form-control" autocomplete="new-password" />
        </div>
        <div class="col-12">
          <label class="form-label">Base DN</label>
          <input v-model="form.base_dn" class="form-control" placeholder="DC=example,DC=com" />
        </div>
        <div class="col-md-4">
          <label class="form-label">用户过滤器</label>
          <input v-model="form.user_filter" class="form-control font-monospace" placeholder="(uid=%s)" />
          <div class="form-hint">%s 将被替换为登录用户名</div>
        </div>
        <div class="col-md-4">
          <label class="form-label">用户名字段</label>
          <input v-model="form.username_attr" class="form-control font-monospace" />
        </div>
        <div class="col-md-4">
          <label class="form-label">邮箱字段</label>
          <input v-model="form.email_attr" class="form-control font-monospace" />
        </div>
        <div class="col-md-6">
          <label class="form-label">测试用户名（可选）</label>
          <input v-model="form.test_username" class="form-control" placeholder="连接测试时试搜该用户" />
        </div>
      </div>
    </div>

    <div class="cd-settings-section">
      <h4>部门 / 用户同步</h4>
      <label class="form-check form-switch mb-3">
        <input v-model="form.sync_enabled" class="form-check-input" type="checkbox" />
        <span class="form-check-label">启用同步（管理端可手动执行；定时任务「LDAP 同步」也会按此开关执行）</span>
      </label>
      <div class="row g-3" :class="{ 'opacity-50': !form.sync_enabled }">
        <div class="col-md-6">
          <label class="form-label">部门 Base DN</label>
          <input v-model="form.dept_base_dn" class="form-control" placeholder="空则用 Base DN" :disabled="!form.sync_enabled" />
        </div>
        <div class="col-md-3">
          <label class="form-label">部门过滤器</label>
          <input v-model="form.dept_filter" class="form-control font-monospace" placeholder="(objectClass=organizationalUnit)" :disabled="!form.sync_enabled" />
        </div>
        <div class="col-md-3">
          <label class="form-label">部门名字段</label>
          <input v-model="form.dept_name_attr" class="form-control font-monospace" placeholder="ou" :disabled="!form.sync_enabled" />
        </div>
        <div class="col-12">
          <label class="form-label">用户同步过滤器</label>
          <input v-model="form.user_sync_filter" class="form-control font-monospace" placeholder="(|(objectClass=inetOrgPerson)(objectClass=person)(objectClass=user))" :disabled="!form.sync_enabled" />
        </div>
      </div>
    </div>

    <div class="cd-settings-actions d-flex flex-wrap gap-2">
      <button type="button" class="btn btn-primary" :disabled="saving" @click="save">
        {{ saving ? '保存中…' : '保存' }}
      </button>
      <button type="button" class="btn btn-outline-secondary" :disabled="testing" @click="testConn">
        {{ testing ? '测试中…' : '测试连接' }}
      </button>
    </div>
  </div>
</template>
