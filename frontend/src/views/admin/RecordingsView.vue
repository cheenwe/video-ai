<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ApiError } from "@/api/client";
import * as api from "@/api/videoAi";
import type { Camera, RecordingPlan } from "@/api/videoAi";
import * as nvrApi from "@/api/nvr";
import type { RecordingFile } from "@/api/nvr";
import { useToast } from "@/composables/useToast";

const toast = useToast();
const cameras = ref<Camera[]>([]);
const selected = ref<Camera | null>(null);
const plan = ref<RecordingPlan | null>(null);
const planSaving = ref(false);
const recordings = ref<RecordingFile[]>([]);
const loadingRec = ref(false);
const playing = ref<string | null>(null);
const probing = ref<Record<number, boolean>>({});
// Event clips are produced by the ingest buffer when an incident fires.
const clips = ref<api.EventClip[]>([]);
const loadingClips = ref(false);
// Platform-side recording (direct RTSP) — separate from the NVR agent list.
const platformRec = ref<api.PlatformRecordings | null>(null);
const loadingPlatform = ref(false);

const isNvrBound = computed(() => !!selected.value?.nvr_agent_id && !!selected.value?.agent_camera_id);

async function load() {
  try {
    const res = await api.listCameras({ page_size: 200 });
    cameras.value = res.items || [];
    if (!selected.value && cameras.value.length) {
      await select(cameras.value[0]);
    }
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  }
}

async function select(c: Camera) {
  selected.value = c;
  recordings.value = [];
  playing.value = null;
  plan.value = null;
  try {
    plan.value = await api.getRecordingPlan(c.id);
  } catch {
    // plan endpoint always answers with a default; ignore errors
  }
  await loadClips();
  platformRec.value = null;
  if (isNvrBound.value) {
    await loadRecordings();
  } else {
    await loadPlatformRecordings();
  }
}

async function loadPlatformRecordings() {
  if (!selected.value) return;
  loadingPlatform.value = true;
  try {
    platformRec.value = await api.cameraRecordings(selected.value.id);
  } catch (e) {
    platformRec.value = null;
    toast.show(e instanceof ApiError ? e.message : "平台录像列表获取失败");
  } finally {
    loadingPlatform.value = false;
  }
}

function playPlatform(seg: api.PlatformSegment) {
  if (!selected.value) return;
  playing.value = api.cameraRecordingURL(selected.value.id, seg.name);
}

function fmtDuration(ms: number) {
  if (!ms) return "录制中";
  const s = Math.round(ms / 1000);
  return s >= 60 ? `${Math.floor(s / 60)} 分 ${s % 60} 秒` : `${s} 秒`;
}

async function loadClips() {
  if (!selected.value) return;
  loadingClips.value = true;
  try {
    clips.value = await api.eventClips(selected.value.id, 30);
  } catch {
    clips.value = [];
  } finally {
    loadingClips.value = false;
  }
}

function clipURL(eventId: number, evidenceId: number) {
  return `/api/events/${eventId}/evidences/${evidenceId}/file`;
}

function evidenceOf(row: api.EventClip, kind: string) {
  return (row.evidences || []).find((e) => e.kind === kind);
}

async function loadRecordings() {
  if (!selected.value?.nvr_agent_id || !selected.value?.agent_camera_id) return;
  loadingRec.value = true;
  try {
    const items = await nvrApi.listRecordings(selected.value.nvr_agent_id, selected.value.agent_camera_id);
    recordings.value = (items || []).slice().reverse();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "录像列表获取失败");
  } finally {
    loadingRec.value = false;
  }
}

function play(file: RecordingFile) {
  if (!selected.value?.nvr_agent_id || !selected.value?.agent_camera_id) return;
  playing.value = nvrApi.recordingURL(selected.value.nvr_agent_id, selected.value.agent_camera_id, file.name);
}

async function savePlan() {
  if (!selected.value || !plan.value) return;
  planSaving.value = true;
  try {
    plan.value = await api.saveRecordingPlan(selected.value.id, plan.value);
    toast.show("录像计划已保存");
    // Reload the segment list right away: its state block reports whether the
    // recorder actually started, so "saved" never implies "recording".
    if (isNvrBound.value) {
      await loadRecordings();
    } else {
      await loadPlatformRecordings();
    }
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  } finally {
    planSaving.value = false;
  }
}

async function probe(c: Camera) {
  probing.value[c.id] = true;
  try {
    const r = await api.probeCamera(c.id);
    c.media_status = r.media_status;
    toast.show(r.ok ? `探测成功：${r.detail || r.media_status}` : `探测失败：${r.error || r.media_status}`);
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "探测失败");
  } finally {
    probing.value[c.id] = false;
  }
}

function fmtSize(n: number) {
  if (n > 1 << 30) return (n / (1 << 30)).toFixed(1) + " GB";
  if (n > 1 << 20) return (n / (1 << 20)).toFixed(1) + " MB";
  return (n / 1024).toFixed(0) + " KB";
}

onMounted(load);
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <h2 class="page-title">录像中心</h2>
    <div class="text-secondary">NVR 通道录像回放与保存策略；探测平台摄像头码流可达性</div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="row g-3">
      <div class="col-lg-4">
        <div class="card">
          <div class="card-header"><h3 class="card-title">摄像头（{{ cameras.length }}）</h3></div>
          <div class="list-group list-group-flush" style="max-height: 65vh; overflow-y: auto">
            <button v-for="c in cameras" :key="c.id" class="list-group-item list-group-item-action"
              :class="{ active: selected?.id === c.id }" @click="select(c)">
              <div class="d-flex justify-content-between align-items-center">
                <span class="text-truncate">{{ c.name }}</span>
                <span class="badge" :class="c.media_status === 'online' ? 'bg-success' : 'bg-secondary'">{{ c.media_status }}</span>
              </div>
              <div class="small text-truncate">{{ c.access_type }}<span v-if="c.agent_camera_id"> / {{ c.agent_camera_id }}</span></div>
            </button>
            <div v-if="!cameras.length" class="list-group-item text-secondary">暂无摄像头，请先在视频管理中登记</div>
          </div>
        </div>
      </div>
      <div class="col-lg-8">
        <template v-if="selected">
          <div class="card mb-3">
            <div class="card-header d-flex justify-content-between align-items-center">
              <h3 class="card-title mb-0">{{ selected.name }} · 录像计划</h3>
              <button class="btn btn-sm" :disabled="probing[selected.id]" @click="probe(selected)">探测码流</button>
            </div>
            <div class="card-body" v-if="plan">
              <div class="row g-2">
                <div class="col-md-3">
                  <label class="form-label">模式</label>
                  <select v-model="plan.mode" class="form-select">
                    <option value="off">关闭</option>
                    <option value="continuous">持续</option>
                    <option value="schedule">按计划时段</option>
                    <option value="event_only">仅事件</option>
                  </select>
                </div>
                <div class="col-md-3">
                  <label class="form-label">保留天数</label>
                  <input v-model.number="plan.retain_days" type="number" min="1" class="form-control" />
                </div>
                <div class="col-md-3">
                  <label class="form-label">容量上限 (GB)</label>
                  <input v-model.number="plan.max_gb" type="number" min="1" class="form-control" />
                </div>
                <div class="col-md-3 d-flex align-items-end">
                  <button class="btn btn-primary" v-permission="'camera:record'" :disabled="planSaving" @click="savePlan">保存计划</button>
                </div>
              </div>
              <div v-if="plan.mode === 'schedule'" class="mt-2">
                <label class="form-label">时段定义（JSON）</label>
                <input v-model="plan.schedule_json" class="form-control" placeholder='{"windows":[{"from":"08:00","to":"18:00","days":[1,2,3,4,5]}]}' />
                <div class="form-hint">days 为 ISO 星期（1=周一…7=周日），省略表示每天；from &gt; to 表示跨零点。</div>
              </div>
              <div v-if="!isNvrBound" class="alert alert-info small mt-2 mb-0">
                该摄像头直连码流：平台用 ffmpeg 流复制分段录制到服务端磁盘（每段 5 分钟），按保留天数与容量上限自动清理。
                「仅事件」不产生连续录像，仅依赖事件片段。
              </div>
            </div>
          </div>

          <div class="card" v-if="!isNvrBound">
            <div class="card-header d-flex justify-content-between align-items-center">
              <h3 class="card-title mb-0">平台录像（直连码流）</h3>
              <button class="btn btn-sm" :disabled="loadingPlatform" @click="loadPlatformRecordings">刷新</button>
            </div>
            <div class="card-body" v-if="platformRec">
              <div class="d-flex flex-wrap gap-3 small">
                <span>
                  状态：
                  <span class="badge" :class="platformRec.state.recording ? 'bg-danger' : platformRec.state.paused ? 'bg-warning text-dark' : 'bg-secondary'">
                    {{ platformRec.state.recording ? "录制中" : platformRec.state.paused ? "时段外暂停" : "未录制" }}
                  </span>
                </span>
                <span>分段 {{ platformRec.state.segments }} 个</span>
                <span>占用 {{ fmtSize(platformRec.state.bytes) }}</span>
                <span class="text-secondary text-truncate" :title="platformRec.state.dir">{{ platformRec.state.dir }}</span>
              </div>
              <div v-if="platformRec.state.last_error" class="alert alert-warning small mt-2 mb-0">
                录制进程：{{ platformRec.state.last_error }}（码流不可达时会每 10 秒重试）
              </div>
            </div>
            <div class="table-responsive">
              <table class="table table-vcenter card-table">
                <thead><tr><th>文件</th><th>大小</th><th>开始时间</th><th>时长</th><th></th></tr></thead>
                <tbody>
                  <tr v-if="loadingPlatform"><td colspan="5" class="text-secondary">加载中…</td></tr>
                  <tr v-else-if="!platformRec || !platformRec.segments.length">
                    <td colspan="5" class="text-secondary">暂无分段。计划为「持续」或「按计划时段」时才会生成。</td>
                  </tr>
                  <tr v-for="s in platformRec?.segments || []" :key="s.name">
                    <td class="text-truncate" style="max-width: 260px">{{ s.name }}</td>
                    <td>{{ fmtSize(s.size_bytes) }}</td>
                    <td>{{ new Date(s.started_at).toLocaleString() }}</td>
                    <td>{{ fmtDuration(s.duration_ms) }}</td>
                    <td class="text-end">
                      <button class="btn btn-sm" @click="playPlatform(s)">播放</button>
                      <a class="btn btn-sm btn-outline-primary ms-1" :href="api.cameraRecordingURL(selected!.id, s.name)" download>下载</a>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="card" v-else>
            <div class="card-header d-flex justify-content-between align-items-center">
              <h3 class="card-title mb-0">录像文件（NVR 通道）</h3>
              <button class="btn btn-sm" :disabled="loadingRec" @click="loadRecordings">刷新</button>
            </div>
            <div class="table-responsive">
              <table class="table table-vcenter card-table">
                <thead><tr><th>文件</th><th>大小</th><th>时间</th><th>状态</th><th></th></tr></thead>
                <tbody>
                  <tr v-if="loadingRec"><td colspan="5" class="text-secondary">加载中…</td></tr>
                  <tr v-else-if="!recordings.length"><td colspan="5" class="text-secondary">暂无录像文件</td></tr>
                  <tr v-for="f in recordings" :key="f.name">
                    <td class="text-truncate" style="max-width: 260px">{{ f.name }}</td>
                    <td>{{ fmtSize(f.size) }}</td>
                    <td>{{ new Date(f.mod_time).toLocaleString() }}</td>
                    <td><span class="badge" :class="f.recording ? 'bg-danger' : 'bg-secondary'">{{ f.recording ? "录制中" : "已完成" }}</span></td>
                    <td class="text-end">
                      <button class="btn btn-sm" @click="play(f)">播放</button>
                      <a class="btn btn-sm btn-outline-primary ms-1" :href="nvrApi.recordingURL(selected!.nvr_agent_id!, selected!.agent_camera_id!, f.name, true)">下载</a>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="card mt-3">
            <div class="card-header d-flex justify-content-between align-items-center">
              <h3 class="card-title mb-0">事件片段（自动截取）</h3>
              <button class="btn btn-sm" :disabled="loadingClips" @click="loadClips">刷新</button>
            </div>
            <div class="card-body" v-if="!clips.length">
              <div class="text-secondary">
                暂无事件片段。事件片段由取流缓冲自动截取（事件前 15 秒 + 后 10 秒），需要摄像头配置码流且服务端取流正常。
              </div>
            </div>
            <div class="table-responsive" v-else>
              <table class="table table-vcenter card-table">
                <thead><tr><th>时间</th><th>类别</th><th>级别</th><th>证据</th><th></th></tr></thead>
                <tbody>
                  <tr v-for="row in clips" :key="row.event.id">
                    <td>{{ new Date(row.event.started_at).toLocaleString() }}</td>
                    <td>{{ row.event.category }}</td>
                    <td><span class="badge" :class="row.event.level === 'critical' ? 'bg-danger' : 'bg-warning text-dark'">{{ row.event.level }}</span></td>
                    <td>
                      <template v-for="e in row.evidences || []" :key="e.id">
                        <span class="badge me-1" :class="e.status === 'ready' ? 'bg-success' : e.status === 'failed' ? 'bg-danger' : 'bg-secondary'"
                          :title="e.error || e.path">{{ e.kind }}·{{ e.status }}</span>
                      </template>
                    </td>
                    <td class="text-end">
                      <template v-if="evidenceOf(row, 'clip')">
                        <a v-if="evidenceOf(row, 'clip')!.status === 'ready'" class="btn btn-sm"
                          :href="clipURL(row.event.id, evidenceOf(row, 'clip')!.id)">下载片段</a>
                      </template>
                      <a v-if="evidenceOf(row, 'screenshot') && evidenceOf(row, 'screenshot')!.status === 'ready'"
                        class="btn btn-sm btn-outline-primary ms-1"
                        :href="clipURL(row.event.id, evidenceOf(row, 'screenshot')!.id)" target="_blank">截图</a>
                      <a v-if="evidenceOf(row, 'overlay')" class="btn btn-sm btn-outline-secondary ms-1"
                        :href="clipURL(row.event.id, evidenceOf(row, 'overlay')!.id)" target="_blank">叠加数据</a>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div v-if="playing" class="card mt-3">
            <div class="card-header d-flex justify-content-between align-items-center">
              <h3 class="card-title mb-0">回放</h3>
              <button class="btn btn-sm btn-link" @click="playing = null">关闭</button>
            </div>
            <div class="card-body">
              <video :src="playing" controls class="w-100" style="max-height: 60vh; background: #000"></video>
            </div>
          </div>
        </template>
        <div v-else class="card"><div class="card-body text-secondary">请选择左侧摄像头</div></div>
      </div>
    </div>
  </div></div>
</template>
