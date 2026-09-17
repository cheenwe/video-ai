<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ApiError } from '@/api/client'
import { useBrandingStore } from '@/stores/branding'
import {
  COLOR_PRESETS,
  DEFAULT_BRANDING,
  LOGIN_FOOTER_POWERED_BY_URL,
  LOGIN_FOOTER_WECHAT,
  DEFAULT_SHARE_AD_URL,
  type BrandingSettings,
} from '@/types/branding'
import { useToast } from '@/composables/useToast'

const branding = useBrandingStore()
const toast = useToast()
const saving = ref(false)

const form = ref<BrandingSettings>({
  ...DEFAULT_BRANDING,
  login_copy: { ...DEFAULT_BRANDING.login_copy },
})

const previewStyle = computed(() => ({
  background: `linear-gradient(135deg, ${form.value.primary_color} 0%, color-mix(in srgb, ${form.value.primary_color} 70%, #0f172a) 100%)`,
}))

function loadForm() {
  form.value = {
    ...DEFAULT_BRANDING,
    ...branding.settings,
    login_copy: { ...DEFAULT_BRANDING.login_copy, ...branding.settings.login_copy },
  }
}

async function save() {
  saving.value = true
  try {
    await branding.save({ ...form.value })
    toast.show('外观品牌已保存')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}

function pickPreset(color: string) {
  form.value.primary_color = color
}

function readImageFile(e: Event, field: 'logo_url' | 'favicon_url') {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  if (file.size > 256 * 1024) {
    toast.show('图片请小于 256KB')
    input.value = ''
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    form.value[field] = String(reader.result || '')
  }
  reader.readAsDataURL(file)
  input.value = ''
}

function clearImage(field: 'logo_url' | 'favicon_url') {
  form.value[field] = ''
}

onMounted(loadForm)
</script>

<template>
  <div class="cd-settings-panel">
    <h3 class="cd-settings-panel-title">外观品牌</h3>
    <p class="text-secondary small mb-4">系统名称、主题色、Logo 与页脚信息。</p>

    <div class="row g-4">
      <div class="col-lg-7">
        <div class="row g-3">
          <div class="col-md-6">
            <label class="form-label">平台名称</label>
            <input v-model="form.site_name" class="form-control" maxlength="32" />
          </div>
          <div class="col-md-6">
            <label class="form-label">副标题</label>
            <input
              v-model="form.tagline"
              class="form-control"
              maxlength="64"
              placeholder="如：统一文件管理 · 智能体"
            />
          </div>
          <div class="col-md-6">
            <label class="form-label">Logo 文字（无图片时显示 1–2 字）</label>
            <input
              v-model="form.logo_text"
              class="form-control"
              maxlength="2"
              placeholder="留空则用默认图标"
            />
          </div>
          <div class="col-md-6">
            <label class="form-label">主题色</label>
            <div class="input-group">
              <input v-model="form.primary_color" type="color" class="form-control form-control-color" />
              <input v-model="form.primary_color" class="form-control font-monospace" maxlength="7" />
            </div>
          </div>
          <div class="col-12">
            <label class="form-label">登录页介绍</label>
            <textarea
              v-model="form.login_description"
              class="form-control"
              rows="2"
              maxlength="256"
              placeholder="登录页左侧简介"
            />
          </div>
        </div>

        <hr class="my-4" />
        <h4 class="mb-3">登录页页脚</h4>
        <div class="row g-3">
          <div class="col-md-6">
            <label class="form-label">名称</label>
            <input
              v-model="form.footer_powered_by_name"
              class="form-control"
              maxlength="32"
              placeholder="Video AI"
            />
          </div>
          <div class="col-md-6">
            <label class="form-label">ICP 备案号</label>
            <input
              v-model="form.footer_icp"
              class="form-control"
              maxlength="64"
              placeholder="如：京ICP备1xx，留空则不显示"
            />
          </div>
          <div class="col-md-6">
            <label class="form-label">页脚链接</label>
            <input
              v-model="form.footer_powered_by_url"
              class="form-control"
              maxlength="512"
              :placeholder="LOGIN_FOOTER_POWERED_BY_URL"
            />
          </div>
          <div class="col-md-6">
            <label class="form-label">微信号</label>
            <input
              v-model="form.footer_wechat"
              class="form-control"
              maxlength="64"
              :placeholder="LOGIN_FOOTER_WECHAT"
            />
          </div>
        </div>

        <div class="mt-3">
          <label class="form-label d-block mb-2">快捷配色</label>
          <div class="cd-color-presets">
            <button
              v-for="p in COLOR_PRESETS"
              :key="p.value"
              type="button"
              class="cd-color-chip"
              :class="{ active: form.primary_color === p.value }"
              :style="{ background: p.value }"
              :title="p.name"
              @click="pickPreset(p.value)"
            />
          </div>
        </div>

        <div class="row g-3 mt-1">
          <div class="col-md-6">
            <label class="form-label">Logo 图片</label>
            <input
              type="file"
              accept="image/*"
              class="form-control form-control-sm"
              @change="readImageFile($event, 'logo_url')"
            />
            <button
              v-if="form.logo_url"
              type="button"
              class="btn btn-sm btn-link px-0 mt-1"
              @click="clearImage('logo_url')"
            >
              清除 Logo
            </button>
          </div>
          <div class="col-md-6">
            <label class="form-label">Favicon</label>
            <input
              type="file"
              accept="image/*"
              class="form-control form-control-sm"
              @change="readImageFile($event, 'favicon_url')"
            />
            <button
              v-if="form.favicon_url"
              type="button"
              class="btn btn-sm btn-link px-0 mt-1"
              @click="clearImage('favicon_url')"
            >
              清除 Favicon
            </button>
          </div>
        </div>

        <div class="cd-settings-actions">
          <button type="button" class="btn btn-primary" :disabled="saving" @click="save">
            {{ saving ? '保存中…' : '保存设置' }}
          </button>
          <button type="button" class="btn" @click="loadForm">重置</button>
        </div>
      </div>

      <div class="col-lg-5">
        <div class="card cd-preview-card overflow-hidden">
          <div class="cd-preview-banner" :style="previewStyle">
            <div class="cd-preview-brand">
              <span v-if="form.logo_url" class="cd-brand-mark me-2">
                <img :src="form.logo_url" alt="" class="cd-brand-img" />
              </span>
              <span v-else-if="form.logo_text" class="cd-brand-mark cd-brand-mark-text me-2">
                {{ form.logo_text.slice(0, 2) }}
              </span>
              <span v-else class="cd-brand-mark cd-brand-mark-dot me-2" :style="{ background: form.primary_color }" />
              <span class="fw-semibold text-white">{{ form.site_name || 'Video AI' }}</span>
            </div>
          </div>
          <div class="card-body">
            <p class="text-secondary small mb-3">实时预览登录页与顶栏效果</p>
            <div class="cd-preview-navbar mb-3">
              <span v-if="form.logo_url" class="cd-brand-mark cd-brand-mark-sm me-2">
                <img :src="form.logo_url" alt="" class="cd-brand-img" />
              </span>
              <span v-else-if="form.logo_text" class="cd-brand-mark cd-brand-mark-sm cd-brand-mark-text me-2">
                {{ form.logo_text.slice(0, 2) }}
              </span>
              <span v-else class="cd-brand-mark cd-brand-mark-sm cd-brand-mark-dot me-2" :style="{ background: form.primary_color }" />
              <span class="fw-semibold">{{ form.site_name || 'Video AI' }}</span>
              <span class="ms-auto badge" :style="{ background: form.primary_color, color: '#fff' }">主题色</span>
            </div>
            <button
              type="button"
              class="btn btn-sm w-100"
              :style="{
                background: form.primary_color,
                borderColor: form.primary_color,
                color: '#fff',
              }"
            >
              主按钮示例
            </button>
            <p v-if="form.tagline" class="text-secondary small mt-3 mb-0">{{ form.tagline }}</p>
          </div>
        </div>
        <p class="text-secondary small mt-2">
          默认页脚链接 {{ LOGIN_FOOTER_POWERED_BY_URL }}；默认微信 {{ LOGIN_FOOTER_WECHAT }}；默认广告跳转 {{ DEFAULT_SHARE_AD_URL }}
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cd-color-presets {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}
.cd-color-chip {
  width: 1.5rem;
  height: 1.5rem;
  border: 2px solid rgba(0, 0, 0, 0.1);
  border-radius: 50%;
  padding: 0;
  cursor: pointer;
  transition: transform 0.15s, border-color 0.15s;
}
.cd-color-chip:hover {
  transform: scale(1.1);
}
.cd-color-chip.active {
  border-color: #000;
}
.cd-preview-banner {
  padding: 1.5rem;
  min-height: 8rem;
  display: flex;
  align-items: center;
}
.cd-preview-brand {
  display: flex;
  align-items: center;
}
.cd-brand-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.2);
  overflow: hidden;
}
.cd-brand-mark-sm {
  width: 1.5rem;
  height: 1.5rem;
}
.cd-brand-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.cd-brand-mark-text {
  font-weight: 600;
  color: #fff;
}
.cd-brand-mark-dot {
  border-radius: 50%;
}
.cd-preview-navbar {
  display: flex;
  align-items: center;
}
</style>
