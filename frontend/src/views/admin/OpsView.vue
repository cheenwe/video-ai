<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { ApiError, fmtDate, fmtSize } from "@/api/client";
import * as opsApi from "@/api/ops";
import type { BackupManifest, BackupStatus, NTPQueryResult } from "@/api/ops";
import { useToast } from "@/composables/useToast";
import { useAuthStore } from "@/stores/auth";
import CdModal from "@/components/CdModal.vue";
import {
  IconRefresh,
  IconDatabase,
  IconClock,
  IconTrash,
  IconDownload,
  IconArrowBackUp,
} from "@tabler/icons-vue";

const toast = useToast();
const auth = useAuthStore();

const loading = ref(false);
const creating = ref(false);
const restoring = ref(false);
const backups = ref<BackupManifest[]>([]);
const backupSt = ref<BackupStatus | null>(null);
const ntpServer = ref("ntp.aliyun.com");
const ntpResult = ref<NTPQueryResult | null>(null);
const ntpQuerying = ref(false);
const showRestore = ref(false);
const restoreTarget = ref<BackupManifest | null>(null);

const now = ref(new Date());
let clockTimer: ReturnType<typeof setInterval> | null = null;

const stateLabel = computed(() => {
  const s = backupSt.value?.state;
  if (s === "prepared") return "恢复目录已准备";
  if (s === "running") return "运行中";
  if (s === "failed") return "失败";
  return "空闲";
});

const stateBadgeClass = computed(() => {
  const s = backupSt.value?.state;
  if (s === "running") return "bg-yellow-lt text-yellow";
  if (s === "failed") return "bg-red-lt text-red";
  return "bg-green-lt text-green";
});

const lastBackupTime = computed(() => backups.value[0]?.created_at || "");

const backupDir = computed(() => {
  const p = backups.value[0]?.archive_path;
  if (!p) return "";
  const idx = p.lastIndexOf("/");
  return idx >= 0 ? p.substring(0, idx) : p;
});

const offsetLabel = computed(() => {
  if (!ntpResult.value) return "—";
  const ms = ntpResult.value.offset_ms;
  const sign = ms >= 0 ? "+" : "-";
  const abs = Math.abs(ms);
  if (abs < 1000) return `${sign}${abs} ms`;
  return `${sign}${(abs / 1000).toFixed(2)} s`;
});

const busy = computed(
  () =>
    creating.value ||
    restoring.value ||
    backupSt.value?.state === "running",
);

function fmtNow(d: Date) {
  const pad = (n: number) => n.toString().padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(
    d.getDate(),
  )} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function fmtDateTime(s: string) {
  if (!s) return "—";
  return fmtNow(new Date(s));
}

async function loadBackups() {
  loading.value = true;
  try {
    const [list, st] = await Promise.all([
      opsApi.listBackups(),
      opsApi.backupStatus(),
    ]);
    backups.value = list.items || [];
    backupSt.value = st;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function loadNtpServer() {
  try {
    const cfg = await opsApi.getGeneralSettings();
    ntpServer.value = (cfg.ntp_server || "ntp.aliyun.com").trim();
  } catch {
    ntpServer.value = "ntp.aliyun.com";
  }
}

async function createBackup() {
  if (!auth.isSuperAdmin) return toast.show("需要超级管理员权限");
  creating.value = true;
  try {
    await opsApi.createBackup();
    toast.show("备份已创建");
    await loadBackups();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "创建失败");
  } finally {
    creating.value = false;
  }
}

async function syncNTP() {
  ntpQuerying.value = true;
  try {
    ntpResult.value = await opsApi.queryNTP(ntpServer.value);
    toast.show("已查询 NTP 时间");
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "NTP 查询失败");
  } finally {
    ntpQuerying.value = false;
  }
}

async function downloadBackup(b: BackupManifest) {
  try {
    await opsApi.downloadBackup(b.id);
    toast.show("已开始下载");
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "下载失败");
  }
}

function openRestore(b: BackupManifest) {
  if (!auth.isSuperAdmin) return toast.show("需要超级管理员权限");
  restoreTarget.value = b;
  showRestore.value = true;
}

async function confirmRestore() {
  if (!restoreTarget.value) return;
  restoring.value = true;
  try {
    await opsApi.restoreBackup(restoreTarget.value.id);
    showRestore.value = false;
    restoreTarget.value = null;
    toast.show("恢复目录已校验，停止服务后切换数据目录再启动");
    await loadBackups();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "恢复失败");
  } finally {
    restoring.value = false;
  }
}

async function removeBackup(b: BackupManifest) {
  if (!auth.isSuperAdmin) return toast.show("需要超级管理员权限");
  if (!confirm(`删除备份 ${b.id}？`)) return;
  try {
    await opsApi.deleteBackup(b.id);
    toast.show("已删除");
    loadBackups();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "删除失败");
  }
}

onMounted(() => {
  clockTimer = setInterval(() => {
    now.value = new Date();
  }, 1000);
  loadNtpServer();
  loadBackups();
});

onUnmounted(() => {
  if (clockTimer) clearInterval(clockTimer);
});
</script>

<template>
  <div>
    <div v-if="backupSt?.restore_path" class="alert alert-info">{{ backupSt.message }}<br/><code>{{ backupSt.restore_path }}</code></div>
    <div class="d-flex gap-2 mb-3 flex-wrap align-items-center">
      <button
        type="button"
        class="btn btn-sm btn-primary"
        :disabled="busy"
        @click="createBackup"
      >
        <IconDatabase :size="16" class="me-1" />创建备份
      </button>
      <button
        type="button"
        class="btn btn-sm"
        :disabled="loading"
        @click="loadBackups"
      >
        <IconRefresh :size="16" class="me-1" />刷新
      </button>
      <span v-if="busy" class="small text-secondary">处理中…</span>
    </div>

    <div class="row g-3 mb-3">
      <div class="col-md-6">
        <div class="card h-100">
          <div class="card-header py-2"><strong>备份服务</strong></div>
          <div class="card-body">
            <dl class="row mb-0 small">
              <dt class="col-5 text-secondary">当前状态</dt>
              <dd class="col-7">
                <span class="badge" :class="stateBadgeClass">{{
                  stateLabel
                }}</span>
                <span
                  v-if="backupSt?.state === 'running'"
                  class="text-secondary ms-2"
                >
                  {{ backupSt.percent }}% · {{ backupSt.message }}
                </span>
              </dd>
              <dt class="col-5 text-secondary">上次备份时间</dt>
              <dd class="col-7">
                {{ lastBackupTime ? fmtDate(lastBackupTime) : "—" }}
              </dd>
              <dt class="col-5 text-secondary">备份目录</dt>
              <dd class="col-7 font-monospace text-break">
                {{ backupDir || "—" }}
              </dd>
              <template v-if="backupSt?.error">
                <dt class="col-5 text-secondary">错误</dt>
                <dd class="col-7 text-danger text-break">
                  {{ backupSt.error }}
                </dd>
              </template>
            </dl>
          </div>
        </div>
      </div>

      <div class="col-md-6">
        <div class="card h-100">
          <div
            class="card-header py-2 d-flex justify-content-between align-items-center"
          >
            <strong>NTP 时间同步</strong>
            <button
              type="button"
              class="btn btn-sm btn-primary"
              :disabled="ntpQuerying"
              @click="syncNTP"
            >
              <IconClock :size="16" class="me-1" />同步
            </button>
          </div>
          <div class="card-body">
            <dl class="row mb-0 small">
              <dt class="col-5 text-secondary">当前本地时间</dt>
              <dd class="col-7 font-monospace">{{ fmtNow(now) }}</dd>
              <dt class="col-5 text-secondary">NTP 服务器时间</dt>
              <dd class="col-7 font-monospace">
                {{ ntpResult ? fmtDateTime(ntpResult.ntp_time) : "—" }}
              </dd>
              <dt class="col-5 text-secondary">偏移量</dt>
              <dd class="col-7">{{ offsetLabel }}</dd>
              <dt class="col-5 text-secondary">往返延迟</dt>
              <dd class="col-7">
                {{ ntpResult ? ntpResult.round_trip_ms + " ms" : "—" }}
              </dd>
              <dt class="col-5 text-secondary">NTP 服务器</dt>
              <dd class="col-7 font-monospace">{{ ntpServer }}</dd>
            </dl>
          </div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-header py-2"><strong>备份列表</strong></div>
      <div class="table-responsive">
        <table class="table table-vcenter card-table cd-admin-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>创建时间</th>
              <th>大小</th>
              <th>数据库</th>
              <th>状态</th>
              <th class="w-1">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="b in backups" :key="b.id">
              <td><code>{{ b.id }}</code></td>
              <td>{{ fmtDate(b.created_at) }}</td>
              <td>{{ fmtSize(b.size_bytes) }}</td>
              <td>
                <span class="badge bg-secondary-lt">{{ b.db_driver || "sqlite" }}</span>
                <div v-if="b.db_dump_error" class="text-danger small mt-1" :title="b.db_dump_error">
                  导出失败
                </div>
              </td>
              <td>
                <span v-if="b.db_dump_error" class="badge bg-warning-lt">部分成功</span>
                <span v-else class="badge bg-secondary-lt">已完成</span>
              </td>
              <td class="text-end text-nowrap">
                <button
                  type="button"
                  class="btn btn-sm btn-ghost-secondary"
                  @click="downloadBackup(b)"
                >
                  <IconDownload :size="16" class="me-1" />下载
                </button>
                <button
                  v-if="auth.isSuperAdmin"
                  type="button"
                  class="btn btn-sm btn-ghost-secondary"
                  :disabled="busy"
                  @click="openRestore(b)"
                >
                  <IconArrowBackUp :size="16" class="me-1" />恢复
                </button>
                <button
                  v-if="auth.isSuperAdmin"
                  type="button"
                  class="btn btn-sm btn-ghost-danger"
                  :disabled="busy"
                  @click="removeBackup(b)"
                >
                  <IconTrash :size="16" class="me-1" />删除
                </button>
              </td>
            </tr>
            <tr v-if="!backups.length && !loading">
              <td colspan="6" class="text-secondary text-center py-4">
                暂无备份
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <CdModal
      :show="showRestore"
      title="准备独立恢复目录"
      size="sm"
      @close="showRestore = false"
    >
      <div class="modal-body">
        <p class="small mb-2">
          恢复备份 <code>{{ restoreTarget?.id }}</code>
          将生成独立的恢复目录供检查与切换。
        </p>
        <p class="small text-danger mb-0">将校验备份并生成新的恢复目录，不会覆盖当前运行数据。准备完成后，停止服务，将 BASE_DATA_DIR 指向恢复目录并重新启动。</p>
      </div>
      <div class="modal-footer">
        <button
          type="button"
          class="btn btn-sm"
          @click="showRestore = false"
        >
          取消
        </button>
        <button
          type="button"
          class="btn btn-sm btn-warning"
          :disabled="restoring"
          @click="confirmRestore"
        >
          校验并准备恢复
        </button>
      </div>
    </CdModal>
  </div>
</template>
