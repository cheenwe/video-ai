<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ApiError } from '@/api/client'
import * as settingsApi from '@/api/settings'
import { useLicenseStore } from '@/stores/license'
import { useToast } from '@/composables/useToast'
import { DEFAULT_WATERMARK, normalizeWatermarkSettings } from '@/utils/watermark'

interface WatermarkSettings {
  enabled: boolean
  text: string
  opacity: number
  font_size: number
  angle: number
  density?: number
  scope: string[]
}

const toast = useToast()
const licenseStore = useLicenseStore()
const watermark = ref<WatermarkSettings>({ ...DEFAULT_WATERMARK })
const saving = ref(false)
const loading = ref(false)

const canEdit = () => licenseStore.hasFeature('watermark')

async function load() {
  if (!canEdit()) return
  loading.value = true
  try {
    watermark.value = normalizeWatermarkSettings(await settingsApi.getSettings<WatermarkSettings>('watermark'))
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!canEdit()) {
    toast.show('需要旗舰版水印授权')
    return
  }
  saving.value = true
  try {
    const payload = normalizeWatermarkSettings(watermark.value)
    const saved = await settingsApi.saveSettings<WatermarkSettings>('watermark', payload)
    // 合并返回值：旧后端若漏回 density，保留本次提交的值
    watermark.value = normalizeWatermarkSettings({ ...payload, ...saved, density: saved.density ?? payload.density })
    await licenseStore.refreshLicense()
    toast.show('水印配置已保存')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="cd-settings-panel">
    <h3 class="cd-settings-panel-title">系统水印</h3>
    <p class="text-secondary small mb-4">预览、分享与下载时的水印样式。</p>

    <p v-if="!canEdit()" class="text-secondary small mb-3">系统水印需旗舰版授权（watermark）。</p>

    <div v-if="loading" class="text-secondary py-4">加载中…</div>
    <template v-else>
      <div class="cd-settings-section">
        <label class="form-check form-switch mb-3">
          <input id="wm-enabled" v-model="watermark.enabled" class="form-check-input" type="checkbox" :disabled="!canEdit()" />
          <span class="form-check-label">启用水印</span>
        </label>
        <div class="mb-3">
          <label class="form-label">水印文字</label>
          <input
            v-model="watermark.text"
            class="form-control"
            placeholder="{username} · {datetime}"
            :disabled="!canEdit()"
          />
          <div class="form-hint">支持变量：{username}、{datetime}</div>
        </div>
        <div class="row g-3">
          <div class="col-md-3">
            <label class="form-label">透明度</label>
            <input
              v-model.number="watermark.opacity"
              type="number"
              step="0.01"
              min="0.01"
              max="0.5"
              class="form-control"
              :disabled="!canEdit()"
            />
          </div>
          <div class="col-md-3">
            <label class="form-label">字号</label>
            <input
              v-model.number="watermark.font_size"
              type="number"
              min="10"
              max="32"
              class="form-control"
              :disabled="!canEdit()"
            />
          </div>
          <div class="col-md-3">
            <label class="form-label">角度</label>
            <input
              v-model.number="watermark.angle"
              type="number"
              min="-90"
              max="90"
              class="form-control"
              :disabled="!canEdit()"
            />
          </div>
          <div class="col-md-3">
            <label class="form-label">密度</label>
            <input
              v-model.number="watermark.density"
              type="number"
              step="0.1"
              min="0.1"
              max="2"
              class="form-control"
              :disabled="!canEdit()"
            />
            <div class="form-hint">1 为标准；0.1～2，越小越稀疏</div>
          </div>
        </div>
      </div>

      <div class="cd-settings-actions">
        <button type="button" class="btn btn-primary" :disabled="saving || !canEdit()" @click="save">
          {{ saving ? '保存中…' : '保存' }}
        </button>
      </div>
    </template>
  </div>
</template>
