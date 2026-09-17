<script setup lang="ts">
import { ref } from 'vue'
import { ApiError } from '@/api/client'
import { saveSettings } from '@/api/settings'
import { useToast } from '@/composables/useToast'
import { usePageSize } from '@/composables/usePageSize'

interface GeneralSettings {
  page_size: number
  local_files_root: string
  ntp_enabled: boolean
  ntp_server: string
}

const toast = useToast()
const { invalidatePageSize } = usePageSize()
const form = defineModel<GeneralSettings>({ required: true })
const saving = ref(false)

async function save() {
  saving.value = true
  try {
    const n = Math.max(1, Math.min(200, form.value.page_size || 20))
    const root = (form.value.local_files_root || '').trim()
    form.value = await saveSettings<GeneralSettings>('general', {
      page_size: n,
      local_files_root: root,
      ntp_enabled: form.value.ntp_enabled,
      ntp_server: (form.value.ntp_server || '').trim(),
    })
    invalidatePageSize()
    toast.show('常规配置已保存')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="cd-settings-panel">
    <h3 class="cd-settings-panel-title">常规设置</h3>
    <p class="text-secondary small mb-4">列表分页、本地文件根目录与 NTP 自动对时。</p>

    <div class="cd-settings-section">
      <h4>系统参数</h4>
      <div class="row g-3">
        <div class="col-md-4">
          <label class="form-label">每页条数</label>
          <input
            v-model.number="form.page_size"
            type="number"
            min="1"
            max="200"
            class="form-control"
          />
          <div class="form-hint">影响用户、日志、存储源等列表（1–200）</div>
        </div>
        <div class="col-md-8">
          <label class="form-label">本地文件根目录</label>
          <input
            v-model="form.local_files_root"
            type="text"
            class="form-control font-monospace"
            placeholder="例如 /data/files 或 D:/"
          />
          <div class="form-hint">仅影响默认「本地存储」的文件浏览根路径，可留空。</div>
        </div>
      </div>
    </div>

    <div class="cd-settings-section">
      <h4>NTP 自动对时</h4>
      <label class="form-check form-switch mb-3">
        <input v-model="form.ntp_enabled" class="form-check-input" type="checkbox" />
        <span class="form-check-label">启用 NTP 自动对时</span>
      </label>
      <div class="row g-3" :class="{ 'opacity-50': !form.ntp_enabled }">
        <div class="col-md-6">
          <label class="form-label">NTP 服务器</label>
          <input
            v-model="form.ntp_server"
            type="text"
            class="form-control font-monospace"
            placeholder="ntp.aliyun.com"
            :disabled="!form.ntp_enabled"
          />
          <div class="form-hint">默认 ntp.aliyun.com；启动后定期校准系统时间。</div>
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
