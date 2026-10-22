<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import {
  IconUsers,
  IconBuilding,
  IconListDetails,
  IconDatabase,
  IconCalendarTime,
  IconRefresh,
  IconActivity,
  IconCpu,
  IconServer2,
  IconClock,
  IconServer,
  IconDeviceCctv,
  IconPlayerPlay,
  IconAlertTriangle,
} from '@tabler/icons-vue'
import { ApiError } from '@/api/client'
import { getAdminOverview } from '@/api/adminOverview'
import type { AdminOverview } from '@/api/adminOverview'
import { useToast } from '@/composables/useToast'

const toast = useToast()
const loading = ref(false)
const data = ref<AdminOverview | null>(null)

const statCards = computed(() => {
  const s = data.value?.stats
  return [
    { key: 'cameras', label: '摄像头', value: s?.cameras ?? 0, icon: IconDeviceCctv, to: '/admin/camera-manage' },
    { key: 'running_deployments', label: '运行布控', value: s?.running_deployments ?? 0, icon: IconPlayerPlay, to: '/admin/deployments' },
    { key: 'pending_events', label: '待处理告警', value: s?.pending_events ?? 0, icon: IconAlertTriangle, to: '/admin/alerts' },
    { key: 'users', label: '平台用户', value: s?.users ?? 0, icon: IconUsers, to: '/admin/users' },
    { key: 'departments', label: '组织部门', value: s?.departments ?? 0, icon: IconBuilding, to: '/admin/departments' },
    { key: 'storages', label: '存储对接', value: s?.storages ?? 0, icon: IconDatabase, to: '/admin/storages' },
    { key: 'schedule_jobs', label: '计划任务', value: s?.schedule_jobs ?? 0, icon: IconCalendarTime, to: '/admin/schedules' },
    { key: 'logs', label: '操作记录', value: s?.logs ?? 0, icon: IconListDetails, to: '/admin/logs' },
  ]
})

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

function formatUptime(sec: number): string {
  if (!sec || sec <= 0) return '—'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const parts: string[] = []
  if (d > 0) parts.push(`${d} 天`)
  if (h > 0) parts.push(`${h} 小时`)
  if (m > 0) parts.push(`${m} 分钟`)
  return parts.join(' ') || '刚刚'
}

function relativeTime(iso: string): string {
  if (!iso) return ''
  const diff = Date.now() - new Date(iso).getTime()
  const sec = Math.max(0, Math.floor(diff / 1000))
  if (sec < 60) return '刚刚'
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min} 分钟前`
  const hour = Math.floor(min / 60)
  if (hour < 24) return `${hour} 小时前`
  const day = Math.floor(hour / 24)
  if (day === 1) return '昨天'
  if (day < 7) return `${day} 天前`
  return new Date(iso).toLocaleDateString('zh-CN')
}

const systemItems = computed(() => {
  const s = data.value?.system
  if (!s) return []
  return [
    { label: '主机名', value: s.hostname || '—', icon: IconServer },
    { label: '操作系统', value: [s.platform, s.os].filter(Boolean).join(' / ') || '—', icon: IconServer },
    { label: 'CPU 使用率', value: `${s.cpu_percent.toFixed(1)}%`, icon: IconCpu, percent: s.cpu_percent, sub: `${s.cpu_cores} 核` },
    { label: '内存使用率', value: `${s.mem_percent.toFixed(1)}%`, icon: IconServer2, percent: s.mem_percent, sub: `${formatBytes(s.mem_used)} / ${formatBytes(s.mem_total)}` },
    { label: '运行时长', value: formatUptime(s.uptime_sec), icon: IconClock },
    { label: 'Go 运行时', value: s.go_version || '—', icon: IconServer },
  ]
})

const diskItems = computed(() => data.value?.system?.disks ?? [])

async function load() {
  loading.value = true
  try {
    data.value = await getAdminOverview()
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="d-flex align-items-center justify-content-between gap-2 mb-4">
      <div>
        <h2>平台概览</h2>
        <p class="text-secondary mb-0">组织、账号与系统运行情况</p>
      </div>
      <button class="btn btn-sm d-inline-flex align-items-center gap-1" :disabled="loading" @click="load">
        <IconRefresh :size="16" :class="{ 'va-spin': loading }" />
        刷新
      </button>
    </div>

    <div v-if="loading && !data" class="text-center text-secondary py-5">加载中…</div>
    <template v-else-if="data">
      <!-- 统计卡片 -->
      <div class="row g-3 mb-4">
        <div v-for="card in statCards" :key="card.key" class="col-md-3 col-sm-6">
          <RouterLink :to="card.to" class="card card-body va-stat-card">
            <div class="d-flex align-items-center justify-content-between">
              <div>
                <div class="text-secondary small mb-1">{{ card.label }}</div>
                <div class="h2 mb-0">{{ card.value }}</div>
              </div>
              <component :is="card.icon" :size="28" class="va-stat-icon" />
            </div>
          </RouterLink>
        </div>
      </div>

      <!-- 系统资源 -->
      <div class="card mb-4">
        <div class="card-header">
          <h3 class="card-title mb-0">
            <IconServer :size="18" class="me-2" />系统资源
          </h3>
          <span class="badge bg-green-lt ms-auto">服务在线</span>
        </div>
        <div class="card-body">
          <div class="row g-4">
            <div v-for="item in systemItems" :key="item.label" class="col-md-4 col-sm-6">
              <div class="d-flex align-items-center gap-3">
                <component :is="item.icon" :size="22" class="text-secondary" />
                <div class="flex-fill">
                  <div class="text-secondary small">{{ item.label }}</div>
                  <div class="fw-bold">{{ item.value }}</div>
                  <div v-if="item.sub" class="text-secondary small">{{ item.sub }}</div>
                </div>
              </div>
              <div v-if="item.percent !== undefined" class="progress mt-2" style="height: 4px">
                <div
                  class="progress-bar"
                  :class="{ 'bg-warning': item.percent > 80, 'bg-danger': item.percent > 90 }"
                  :style="{ width: `${Math.min(100, item.percent)}%` }"
                />
              </div>
            </div>
          </div>

          <!-- 磁盘使用 -->
          <div v-if="diskItems.length" class="mt-4">
            <div class="text-secondary small mb-2"><IconDatabase :size="16" class="me-1" />磁盘使用</div>
            <div v-for="disk in diskItems" :key="disk.path" class="mb-2">
              <div class="d-flex justify-content-between small">
                <span class="font-monospace">{{ disk.path }}</span>
                <span>{{ formatBytes(disk.used) }} / {{ formatBytes(disk.total) }} ({{ disk.percent.toFixed(1) }}%)</span>
              </div>
              <div class="progress" style="height: 6px">
                <div
                  class="progress-bar"
                  :class="{ 'bg-warning': disk.percent > 80, 'bg-danger': disk.percent > 90 }"
                  :style="{ width: `${Math.min(100, disk.percent)}%` }"
                />
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 趋势图 + 最近活动 -->
      <div class="row g-4">
        <div class="col-lg-7">
          <div class="card h-100">
            <div class="card-header">
              <h3 class="card-title mb-0">
                <IconActivity :size="18" class="me-2" />12 天操作趋势
              </h3>
            </div>
            <div class="card-body">
              <div class="va-chart-bars">
                <div
                  v-for="point in data.chart"
                  :key="point.label"
                  class="va-bar-wrap"
                  :title="`${point.label}: ${point.count} 条`"
                >
                  <div class="va-bar" :style="{ height: `${point.height}%` }">
                    <span v-if="point.count > 0" class="va-bar-count">{{ point.count }}</span>
                  </div>
                  <span class="va-bar-label">{{ point.label }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div class="col-lg-5">
          <div class="card h-100">
            <div class="card-header d-flex align-items-center justify-content-between">
              <h3 class="card-title mb-0">
                <IconListDetails :size="18" class="me-2" />最近活动
              </h3>
              <RouterLink to="/admin/logs" class="small">查看全部</RouterLink>
            </div>
            <div class="card-body va-activity-list">
              <div v-if="!data.activity.length" class="text-secondary small text-center py-3">
                暂无活动记录
              </div>
              <div v-for="item in data.activity" :key="item.id" class="va-activity-item">
                <div class="d-flex justify-content-between align-items-start">
                  <span class="va-activity-action">{{ item.action }}</span>
                  <span class="va-activity-time">{{ relativeTime(item.created_at) }}</span>
                </div>
                <div class="va-activity-user">
                  <span class="badge" :class="item.status === 'success' ? 'bg-green-lt' : 'bg-red-lt'">
                    {{ item.status === 'success' ? '成功' : '失败' }}
                  </span>
                  {{ item.username }}
                  <span v-if="item.remark" class="text-secondary small">· {{ item.remark }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.va-stat-card {
  text-decoration: none;
  color: inherit;
  transition: box-shadow 0.15s ease, transform 0.15s ease;
}
.va-stat-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
  transform: translateY(-1px);
}
.va-stat-icon {
  color: var(--tblr-primary);
  opacity: 0.6;
}
.va-spin {
  animation: va-spin 1s linear infinite;
}
@keyframes va-spin {
  to { transform: rotate(360deg); }
}
.va-chart-bars {
  display: flex;
  align-items: flex-end;
  gap: 4px;
  height: 180px;
  padding-top: 20px;
}
.va-bar-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  height: 100%;
  min-width: 0;
}
.va-bar {
  width: 100%;
  max-width: 32px;
  background: var(--tblr-primary);
  opacity: 0.7;
  border-radius: 3px 3px 0 0;
  margin: auto auto 0;
  position: relative;
  min-height: 2px;
  transition: opacity 0.15s ease;
}
.va-bar:hover {
  opacity: 1;
}
.va-bar-count {
  position: absolute;
  top: -18px;
  left: 50%;
  transform: translateX(-50%);
  font-size: 10px;
  color: #627081;
}
.va-bar-label {
  font-size: 10px;
  color: #8492a3;
  margin-top: 6px;
}
.va-activity-list {
  max-height: 320px;
  overflow-y: auto;
}
.va-activity-item {
  padding: 10px 0;
  border-bottom: 1px solid #f0f2f5;
}
.va-activity-item:last-child {
  border-bottom: none;
}
.va-activity-action {
  font-weight: 500;
  font-size: 13px;
  word-break: break-all;
}
.va-activity-time {
  font-size: 11px;
  color: #8492a3;
  white-space: nowrap;
  margin-left: 8px;
}
.va-activity-user {
  font-size: 12px;
  color: #627081;
  margin-top: 4px;
}
</style>
