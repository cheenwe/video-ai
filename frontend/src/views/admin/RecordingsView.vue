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
  if (isNvrBound.value) {
    await loadRecordings();
  }
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
    toast.show("录像计划已保存（NVR 通道由 nvr-agent 执行录制；直连 RTSP 录制流水线尚未接入）");
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
                  <select v-model="plan.mode" class="form-select" :disabled="!isNvrBound">
                    <option value="off">关闭</option>
                    <option value="continuous">持续</option>
                    <option value="schedule">计划</option>
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
              <div v-if="!isNvrBound" class="alert alert-info small mt-2 mb-0">
                该摄像头未绑定 NVR 通道。录像计划可保存，但当前录制执行依赖 nvr-agent；直连 RTSP 的平台录像流水线尚未接入。
              </div>
            </div>
          </div>

          <div class="card">
            <div class="card-header d-flex justify-content-between align-items-center">
              <h3 class="card-title mb-0">录像文件（NVR 通道）</h3>
              <button v-if="isNvrBound" class="btn btn-sm" :disabled="loadingRec" @click="loadRecordings">刷新</button>
            </div>
            <div class="card-body" v-if="!isNvrBound">
              <div class="text-secondary">非 NVR 通道摄像头暂无录像来源。</div>
            </div>
            <div class="table-responsive" v-else>
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
