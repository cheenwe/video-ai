<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import type Hls from "hls.js";
import {
  IconRefresh, IconPlayerPlay, IconPlayerStop, IconPlayerRecord, IconDownload, IconSearch, IconVideo,
} from "@tabler/icons-vue";
import { ApiError, fmtDate, fmtSize } from "@/api/client";
import * as nvrApi from "@/api/nvr";
import type { AgentCameras, ArchiveJob, ArchiveSegment, CameraStatus, ProcessStatus, RecordingFile } from "@/api/nvr";
import { useToast } from "@/composables/useToast";
import { useAuthStore } from "@/stores/auth";

const toast = useToast();
const auth = useAuthStore();
const errMsg = (e: unknown, fallback: string) => (e instanceof ApiError ? e.message : fallback);

const groups = ref<AgentCameras[]>([]);
const loading = ref(false);
const selected = ref<{ agentId: number; camId: string } | null>(null);

const current = computed<CameraStatus | null>(() => {
  if (!selected.value) return null;
  const g = groups.value.find((x) => x.agent_id === selected.value!.agentId);
  return g?.cameras.find((c) => c.id === selected.value!.camId) || null;
});
const currentAgent = computed(() => groups.value.find((x) => x.agent_id === selected.value?.agentId) || null);

async function loadCameras(silent = false) {
  if (!silent) loading.value = true;
  try {
    groups.value = (await nvrApi.listCameras()).items;
  } catch (e) {
    if (!silent) toast.show(errMsg(e, "加载失败"));
  } finally {
    loading.value = false;
  }
}

function procBadge(st: ProcessStatus | null, onText: string) {
  if (!st) return { cls: "bg-secondary-lt", text: "停止", title: "" };
  if (st.running) return { cls: "bg-green-lt", text: st.restarts ? `${onText}（重连 ${st.restarts} 次）` : onText, title: st.last_error || "" };
  return { cls: "bg-orange-lt", text: "重连中", title: st.last_error || "" };
}

// ---------- 播放器 ----------
const video = ref<HTMLVideoElement | null>(null);
const playerMode = ref<"none" | "live" | "file">("none");
const playerTitle = ref("");
const playerError = ref("");
const playerLoading = ref(false);
let hls: Hls | null = null;

function stopPlayer() {
  if (hls) {
    hls.destroy();
    hls = null;
  }
  if (video.value) {
    video.value.pause();
    video.value.removeAttribute("src");
    video.value.load();
  }
  playerMode.value = "none";
  playerError.value = "";
  playerLoading.value = false;
}

async function playLive() {
  if (!selected.value || !current.value) return;
  stopPlayer();
  playerMode.value = "live";
  playerTitle.value = `实时预览 · ${current.value.name || current.value.id}`;
  playerLoading.value = true;
  await nextTick();
  const el = video.value!;
  const url = nvrApi.liveURL(selected.value.agentId, selected.value.camId);
  const { default: HlsCtor } = await import("hls.js");
  if (HlsCtor.isSupported()) {
    const h = new HlsCtor({
      enableWorker: false,
      liveSyncDurationCount: 2,
      manifestLoadPolicy: {
        default: { maxTimeToFirstByteMs: 25000, maxLoadTimeMs: 30000, timeoutRetry: { maxNumRetry: 2, retryDelayMs: 1000, maxRetryDelayMs: 4000 }, errorRetry: { maxNumRetry: 2, retryDelayMs: 1000, maxRetryDelayMs: 4000 } },
      },
    });
    hls = h;
    h.on(HlsCtor.Events.MANIFEST_PARSED, () => {
      playerLoading.value = false;
      el.play().catch(() => undefined);
    });
    h.on(HlsCtor.Events.ERROR, (_evt, data) => {
      if (!data.fatal) return;
      playerLoading.value = false;
      const status = data.response?.code;
      playerError.value = status ? `预览失败（HTTP ${status}），请确认摄像头在线、码流可用` : `预览失败：${data.details}`;
      h.destroy();
      if (hls === h) hls = null;
    });
    h.loadSource(url);
    h.attachMedia(el);
  } else if (el.canPlayType("application/vnd.apple.mpegurl")) {
    el.src = url;
    el.onloadeddata = () => (playerLoading.value = false);
    el.play().catch(() => undefined);
  } else {
    playerLoading.value = false;
    playerError.value = "当前浏览器不支持 HLS 播放";
  }
  setTimeout(() => loadCameras(true), 3000);
}

async function playFile(f: RecordingFile) {
  if (!selected.value) return;
  stopPlayer();
  playerMode.value = "file";
  playerTitle.value = `录像回放 · ${f.name}`;
  await nextTick();
  const el = video.value!;
  el.onerror = () => (playerError.value = "录像无法播放（文件可能仍在写入或编码不受浏览器支持，可下载后用播放器打开）");
  el.src = nvrApi.recordingURL(selected.value.agentId, selected.value.camId, f.name);
  el.play().catch(() => undefined);
}

// ---------- 摄像头选择 ----------
function selectCamera(g: AgentCameras, c: CameraStatus) {
  if (selected.value?.agentId === g.agent_id && selected.value.camId === c.id) return;
  stopPlayer();
  selected.value = { agentId: g.agent_id, camId: c.id };
  recordings.value = [];
  segments.value = null;
  archiveJob.value = null;
  if (auth.can("camera:live")) playLive();
  if (auth.can("recording:list")) {
    loadRecordings();
    refreshArchive();
  }
}

// ---------- 录制 ----------
const recordBusy = ref(false);
async function toggleRecord() {
  if (!selected.value || !current.value) return;
  const action = current.value.record ? "stop" : "start";
  recordBusy.value = true;
  try {
    await nvrApi.recordControl(selected.value.agentId, selected.value.camId, action);
    toast.show(action === "start" ? "已开始录制" : "已停止录制");
    await loadCameras(true);
    setTimeout(loadRecordings, 1500);
  } catch (e) {
    toast.show(errMsg(e, "操作失败"));
  } finally {
    recordBusy.value = false;
  }
}

// ---------- 本地录像 ----------
const tab = ref<"local" | "archive">("local");
const recordings = ref<RecordingFile[]>([]);
const recLoading = ref(false);
async function loadRecordings() {
  if (!selected.value) return;
  recLoading.value = true;
  try {
    recordings.value = (await nvrApi.listRecordings(selected.value.agentId, selected.value.camId)) || [];
  } catch (e) {
    toast.show(errMsg(e, "录像列表加载失败"));
  } finally {
    recLoading.value = false;
  }
}

function downloadURL(f: RecordingFile) {
  return selected.value ? nvrApi.recordingURL(selected.value.agentId, selected.value.camId, f.name, true) : "#";
}

// ---------- NVR 历史录像 ----------
function localInput(d: Date) {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
const now = new Date();
const rangeStart = ref(localInput(new Date(now.getTime() - 3600_000)));
const rangeEnd = ref(localInput(now));
const segments = ref<ArchiveSegment[] | null>(null);
const searching = ref(false);
const archiveJob = ref<ArchiveJob | null>(null);
let archiveTimer: number | undefined;

function toISO(local: string) {
  const d = new Date(local);
  return isNaN(d.getTime()) ? "" : d.toISOString();
}

async function searchSegments() {
  if (!selected.value) return;
  const start = toISO(rangeStart.value), end = toISO(rangeEnd.value);
  if (!start || !end || start >= end) {
    toast.show("请选择有效的时间范围");
    return;
  }
  searching.value = true;
  try {
    segments.value = (await nvrApi.searchArchive(selected.value.agentId, selected.value.camId, start, end)) || [];
  } catch (e) {
    toast.show(errMsg(e, "查询失败"));
  } finally {
    searching.value = false;
  }
}

async function downloadArchive(start: string, end: string) {
  if (!selected.value) return;
  try {
    archiveJob.value = await nvrApi.startArchive(selected.value.agentId, selected.value.camId, start, end);
    toast.show("已开始从 NVR 下载，完成后出现在「本地录像」中");
    pollArchive();
  } catch (e) {
    toast.show(errMsg(e, "下载失败"));
  }
}

function downloadRange() {
  const start = toISO(rangeStart.value), end = toISO(rangeEnd.value);
  if (!start || !end || start >= end) {
    toast.show("请选择有效的时间范围");
    return;
  }
  downloadArchive(start, end);
}

async function refreshArchive() {
  if (!selected.value) return;
  try {
    const was = archiveJob.value?.running;
    archiveJob.value = await nvrApi.archiveStatus(selected.value.agentId, selected.value.camId);
    if (was && archiveJob.value && !archiveJob.value.running) loadRecordings();
  } catch {
    /* 状态查询失败不打断操作 */
  }
}

function pollArchive() {
  window.clearInterval(archiveTimer);
  archiveTimer = window.setInterval(async () => {
    await refreshArchive();
    if (!archiveJob.value?.running) window.clearInterval(archiveTimer);
  }, 3000);
}

let statusTimer: number | undefined;
onMounted(() => {
  loadCameras();
  statusTimer = window.setInterval(() => loadCameras(true), 5000);
});
onUnmounted(() => {
  window.clearInterval(statusTimer);
  window.clearInterval(archiveTimer);
  stopPlayer();
});
</script>

<template>
  <div>
    <div class="d-flex align-items-center justify-content-between gap-2 mb-4">
      <div>
        <h2>视频监控</h2>
        <p class="text-secondary mb-0">实时预览、录制控制、本地录像回放与 NVR 历史录像下载</p>
      </div>
      <button class="btn btn-sm" @click="loadCameras()"><IconRefresh :size="15" class="me-1" />刷新</button>
    </div>

    <div class="row g-4">
      <div class="col-xl-3 col-lg-4">
        <div class="card">
          <div class="card-header"><h3 class="card-title mb-0">摄像头</h3></div>
          <div class="card-body p-0">
            <div v-if="loading && !groups.length" class="text-secondary text-center py-4">加载中…</div>
            <div v-else-if="!groups.length" class="text-secondary p-3">暂无可用的 nvr-agent，请先在「NVR 接入」中登记并配置摄像头。</div>
            <div v-for="g in groups" :key="g.agent_id" class="border-bottom">
              <div class="px-3 py-2 d-flex align-items-center justify-content-between bg-body-tertiary">
                <strong class="small">{{ g.agent_name }}</strong>
                <span class="badge" :class="g.online ? 'bg-green-lt' : 'bg-red-lt'">{{ g.online ? "在线" : "离线" }}</span>
              </div>
              <div v-if="g.error" class="small text-danger px-3 py-2">{{ g.error }}</div>
              <div v-else-if="!g.cameras.length" class="small text-secondary px-3 py-2">该节点未配置摄像头</div>
              <div class="list-group list-group-flush">
                <button
                  v-for="c in g.cameras"
                  :key="c.id"
                  class="list-group-item list-group-item-action"
                  :class="{ active: selected?.agentId === g.agent_id && selected?.camId === c.id }"
                  @click="selectCamera(g, c)"
                >
                  <div class="d-flex align-items-center gap-2">
                    <IconVideo :size="16" />
                    <span class="fw-semibold text-truncate">{{ c.name || c.id }}</span>
                  </div>
                  <div class="small text-secondary">{{ c.nvr ? `${c.nvr} / 通道 ${c.channel}` : "自定义地址" }}</div>
                  <div class="small mt-1 d-flex gap-1 flex-wrap">
                    <span class="badge" :class="procBadge(c.live, '预览中').cls" :title="procBadge(c.live, '预览中').title">
                      {{ c.live ? procBadge(c.live, "预览中").text : "未预览" }}
                    </span>
                    <span class="badge" :class="c.record ? procBadge(c.record, '录制中').cls : 'bg-secondary-lt'" :title="procBadge(c.record, '录制中').title">
                      {{ c.record ? procBadge(c.record, "录制中").text : "未录制" }}
                    </span>
                  </div>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="col-xl-9 col-lg-8">
        <div v-if="!current" class="card"><div class="card-body text-secondary text-center py-5">请选择左侧摄像头</div></div>
        <template v-else>
          <div class="card mb-3">
            <div class="card-header d-flex align-items-center justify-content-between gap-2 flex-wrap">
              <h3 class="card-title mb-0">{{ playerMode === "none" ? (current.name || current.id) : playerTitle }}</h3>
              <div class="d-flex gap-2">
                <button v-if="playerMode !== 'live'" class="btn btn-sm" v-permission="'camera:live'" @click="playLive">
                  <IconPlayerPlay :size="15" class="me-1" />实时预览
                </button>
                <button v-if="playerMode !== 'none'" class="btn btn-sm" @click="stopPlayer">
                  <IconPlayerStop :size="15" class="me-1" />停止播放
                </button>
                <button
                  class="btn btn-sm"
                  :class="current.record ? 'btn-danger' : 'btn-outline-danger'"
                  :disabled="recordBusy || !currentAgent?.online"
                  v-permission="'camera:record'"
                  @click="toggleRecord"
                >
                  <IconPlayerRecord :size="15" class="me-1" />{{ current.record ? "停止录制" : "开始录制" }}
                </button>
              </div>
            </div>
            <div class="card-body p-0 position-relative bg-black" style="aspect-ratio: 16 / 9">
              <video ref="video" class="w-100 h-100" controls muted playsinline style="object-fit: contain" />
              <div v-if="playerLoading" class="position-absolute top-50 start-50 translate-middle text-white">正在连接视频流…（首次可能需要 5~10 秒）</div>
              <div v-if="playerError" class="position-absolute top-50 start-50 translate-middle text-white text-center px-3">{{ playerError }}</div>
              <div v-if="playerMode === 'none' && !playerError" class="position-absolute top-50 start-50 translate-middle text-secondary">未播放</div>
            </div>
            <div v-if="current.record?.last_error || current.live?.last_error" class="card-footer small text-secondary">
              <div v-if="current.live?.last_error">预览最近错误：{{ current.live.last_error }}</div>
              <div v-if="current.record?.last_error">录制最近错误：{{ current.record.last_error }}</div>
            </div>
          </div>

          <div class="card" v-permission="'recording:list'">
            <div class="card-header">
              <ul class="nav nav-tabs card-header-tabs">
                <li class="nav-item"><a href="#" class="nav-link" :class="{ active: tab === 'local' }" @click.prevent="tab = 'local'">本地录像</a></li>
                <li class="nav-item"><a href="#" class="nav-link" :class="{ active: tab === 'archive' }" @click.prevent="tab = 'archive'">NVR 历史录像</a></li>
              </ul>
            </div>
            <div class="card-body">
              <template v-if="tab === 'local'">
                <div class="d-flex justify-content-between align-items-center mb-2">
                  <span class="small text-secondary">保存在 nvr-agent 所在电脑上，按分片显示（最新在前）</span>
                  <button class="btn btn-sm btn-ghost-secondary" @click="loadRecordings"><IconRefresh :size="15" /></button>
                </div>
                <div v-if="recLoading" class="text-secondary text-center py-3">加载中…</div>
                <div v-else-if="!recordings.length" class="text-secondary text-center py-3">暂无录像</div>
                <div v-else class="table-responsive" style="max-height: 360px">
                  <table class="table table-sm align-middle mb-0">
                    <thead><tr><th>文件</th><th>大小</th><th>修改时间</th><th></th></tr></thead>
                    <tbody>
                      <tr v-for="f in recordings" :key="f.name">
                        <td><code class="small">{{ f.name }}</code> <span v-if="f.recording" class="badge bg-red-lt ms-1">写入中</span></td>
                        <td>{{ fmtSize(f.size) }}</td>
                        <td class="small text-secondary">{{ fmtDate(f.mod_time) }}</td>
                        <td class="text-end text-nowrap">
                          <button class="btn btn-sm btn-icon" title="播放" @click="playFile(f)"><IconPlayerPlay :size="15" /></button>
                          <a v-if="auth.can('recording:download')" class="btn btn-sm btn-icon" title="下载" :href="downloadURL(f)"><IconDownload :size="15" /></a>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </template>

              <template v-else>
                <div class="row g-2 align-items-end mb-3">
                  <div class="col-md-4"><label class="form-label">开始时间</label><input v-model="rangeStart" type="datetime-local" class="form-control form-control-sm" /></div>
                  <div class="col-md-4"><label class="form-label">结束时间</label><input v-model="rangeEnd" type="datetime-local" class="form-control form-control-sm" /></div>
                  <div class="col-md-4 d-flex gap-2">
                    <button class="btn btn-sm btn-primary" :disabled="searching" @click="searchSegments"><IconSearch :size="15" class="me-1" />{{ searching ? "查询中…" : "查询" }}</button>
                    <button class="btn btn-sm" :disabled="!!archiveJob?.running" v-permission="'camera:record'" @click="downloadRange"><IconDownload :size="15" class="me-1" />下载该时段</button>
                  </div>
                </div>
                <div v-if="archiveJob" class="alert py-2" :class="archiveJob.running ? 'alert-info' : archiveJob.error ? 'alert-danger' : 'alert-success'">
                  <span v-if="archiveJob.running">正在从 NVR 下载 {{ fmtDate(archiveJob.start) }} ~ {{ fmtDate(archiveJob.end) }}…</span>
                  <span v-else-if="archiveJob.error">下载失败：{{ archiveJob.error }}</span>
                  <span v-else>已下载：{{ archiveJob.file }}（见「本地录像」）</span>
                </div>
                <div v-if="segments === null" class="text-secondary small">选择时间范围后点击「查询」，从 NVR 检索该摄像头的录像片段（单次下载最长 6 小时）。</div>
                <div v-else-if="!segments.length" class="text-secondary text-center py-3">该时段 NVR 上没有录像</div>
                <div v-else class="table-responsive" style="max-height: 360px">
                  <table class="table table-sm align-middle mb-0">
                    <thead><tr><th>开始</th><th>结束</th><th></th></tr></thead>
                    <tbody>
                      <tr v-for="s in segments" :key="s.start + s.end">
                        <td>{{ fmtDate(s.start) }}</td>
                        <td>{{ fmtDate(s.end) }}</td>
                        <td class="text-end">
                          <button class="btn btn-sm" :disabled="!!archiveJob?.running" v-permission="'camera:record'" @click="downloadArchive(s.start, s.end)">
                            <IconDownload :size="15" class="me-1" />下载到本地
                          </button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </template>
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
