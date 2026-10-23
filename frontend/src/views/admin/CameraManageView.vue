<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ApiError } from "@/api/client";
import * as api from "@/api/videoAi";
import type { Camera, CameraGroup } from "@/api/videoAi";
import { useToast } from "@/composables/useToast";
import CdModal from "@/components/CdModal.vue";

const toast = useToast();
const loading = ref(false);
const items = ref<Camera[]>([]);
const groups = ref<CameraGroup[]>([]);
const total = ref(0);
const keyword = ref("");
const show = ref(false);
const form = ref({
  code: "", name: "", access_type: "nvr_agent", nvr_agent_id: "" as string | number,
  agent_camera_id: "", main_url: "", sub_url: "", remark: "", group_id: "" as string | number,
});
const editId = ref<number | null>(null);

async function load() {
  loading.value = true;
  try {
    const [cams, gs] = await Promise.all([
      api.listCameras({ keyword: keyword.value, page: 1, page_size: 50 }),
      api.listCameraGroups(),
    ]);
    items.value = cams.items || [];
    total.value = cams.total || 0;
    groups.value = gs || [];
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editId.value = null;
  form.value = { code: "", name: "", access_type: "nvr_agent", nvr_agent_id: "", agent_camera_id: "", main_url: "", sub_url: "", remark: "", group_id: "" };
  show.value = true;
}

function openEdit(c: Camera) {
  editId.value = c.id;
  const main = c.streams?.find((s) => s.role === "main")?.url || "";
  const sub = c.streams?.find((s) => s.role === "sub")?.url || "";
  form.value = {
    code: c.code, name: c.name, access_type: c.access_type,
    nvr_agent_id: c.nvr_agent_id || "", agent_camera_id: c.agent_camera_id || "",
    main_url: main, sub_url: sub, remark: c.remark || "", group_id: c.group_id || "",
  };
  show.value = true;
}

async function save() {
  try {
    const body: Record<string, unknown> = {
      code: form.value.code, name: form.value.name, access_type: form.value.access_type,
      agent_camera_id: form.value.agent_camera_id, main_url: form.value.main_url,
      sub_url: form.value.sub_url, remark: form.value.remark,
    };
    if (form.value.nvr_agent_id) body.nvr_agent_id = Number(form.value.nvr_agent_id);
    if (form.value.group_id) body.group_id = Number(form.value.group_id);
    if (editId.value) await api.updateCamera(editId.value, body);
    else await api.createCamera(body);
    show.value = false;
    toast.show("已保存");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  }
}

async function archive(c: Camera) {
  if (!confirm(`归档摄像头 ${c.name}？`)) return;
  try {
    await api.archiveCamera(c.id);
    toast.show("已归档");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "归档失败");
  }
}

const probing = ref<Record<number, boolean>>({});

async function probe(c: Camera) {
  probing.value[c.id] = true;
  try {
    const r = await api.probeCamera(c.id);
    c.media_status = r.media_status;
    c.device_status = r.ok ? "online" : "offline";
    toast.show(r.ok ? `探测成功：${r.detail || r.media_status}` : `探测失败：${r.error || r.media_status}`);
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "探测失败");
  } finally {
    probing.value[c.id] = false;
  }
}

// ------------------------------------------------------- bulk import ----

const showImport = ref(false);
const importText = ref("");
const importing = ref(false);
const importResult = ref<api.ImportResult[] | null>(null);
const importTally = ref<{ total: number; created: number; failed: number } | null>(null);

const importSample = `code,name,access_type,main_url,sub_url,remark
cam-a,一号机,rtsp,rtsp://192.168.1.10:554/1,,东坑
cam-b,二号机,rtsp,rtsp://192.168.1.11:554/1,,西坑`;

function parseCSV(text: string): api.ImportRow[] {
  const rows: api.ImportRow[] = [];
  text.split(/\r?\n/).forEach((line, lineNo) => {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) return;
    // A CSV header is not a camera.
    if (lineNo === 0 && /^code\s*,/i.test(trimmed)) return;
    // Split outside quotes so remarks containing commas survive.
    const cells: string[] = [];
    let cur = "";
    let quoted = false;
    for (let i = 0; i < trimmed.length; i++) {
      const ch = trimmed[i];
      if (ch === '"') { quoted = !quoted; continue; }
      if (ch === "," && !quoted) { cells.push(cur.trim()); cur = ""; continue; }
      cur += ch;
    }
    cells.push(cur.trim());
    if (cells.length < 2) return;
    rows.push({
      code: cells[0] || "", name: cells[1] || "",
      access_type: cells[2] || "rtsp",
      main_url: cells[3] || "", sub_url: cells[4] || "", remark: cells[5] || "",
    });
  });
  return rows;
}

async function runImport() {
  const rows = parseCSV(importText.value);
  if (!rows.length) {
    toast.show("没有可导入的数据行");
    return;
  }
  importing.value = true;
  importResult.value = null;
  importTally.value = null;
  try {
    const res = await api.importCameras(rows);
    importResult.value = res.results || [];
    importTally.value = { total: res.total, created: res.created, failed: res.failed };
    toast.show(`导入完成：成功 ${res.created}，失败 ${res.failed}`);
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "导入失败");
  } finally {
    importing.value = false;
  }
}

function openImport() {
  importText.value = importSample;
  importResult.value = null;
  importTally.value = null;
  showImport.value = true;
}

onMounted(load);
</script>

<template>
  <div class="page-header d-print-none">
    <div class="container-xl">
      <div class="row g-2 align-items-center">
        <div class="col"><h2 class="page-title">视频管理</h2><div class="text-secondary">平台摄像头实体，可绑定 NVR 通道或直连 RTSP</div></div>
        <div class="col-auto">
          <button class="btn" v-permission="'camera:create'" @click="openImport">批量导入</button>
          <button class="btn btn-primary ms-1" v-permission="'camera:create'" @click="openCreate">新增摄像头</button>
        </div>
      </div>
    </div>
  </div>
  <div class="page-body"><div class="container-xl">
    <div class="card">
      <div class="card-header"><div class="row w-100 g-2">
        <div class="col"><input v-model="keyword" class="form-control" placeholder="搜索名称/编号" @keyup.enter="load" /></div>
        <div class="col-auto"><button class="btn" @click="load">查询</button></div>
      </div></div>
      <div class="table-responsive">
        <table class="table table-vcenter card-table">
          <thead><tr><th>编号</th><th>名称</th><th>接入</th><th>设备</th><th>媒体</th><th>分析</th><th></th></tr></thead>
          <tbody>
            <tr v-if="loading"><td colspan="7" class="text-secondary">加载中…</td></tr>
            <tr v-else-if="!items.length"><td colspan="7" class="text-secondary">暂无摄像头</td></tr>
            <tr v-for="c in items" :key="c.id">
              <td>{{ c.code }}</td><td>{{ c.name }}</td>
              <td>{{ c.access_type }}<span v-if="c.agent_camera_id" class="text-secondary"> / {{ c.agent_camera_id }}</span></td>
              <td>{{ c.device_status }}</td><td>{{ c.media_status }}</td><td>{{ c.analysis_status }}</td>
              <td class="text-end">
                <button class="btn btn-sm" :disabled="probing[c.id]" v-permission="'camera:list'" @click="probe(c)">探测</button>
                <button class="btn btn-sm" v-permission="'camera:update'" @click="openEdit(c)">编辑</button>
                <button class="btn btn-sm btn-outline-danger ms-1" v-permission="'camera:delete'" @click="archive(c)">归档</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="card-footer text-secondary">共 {{ total }} 路</div>
    </div>
  </div></div>

  <CdModal :show="show" :title="editId ? '编辑摄像头' : '新增摄像头'" @close="show = false">
    <div class="modal-body">
      <div class="mb-2"><label class="form-label">编号</label><input v-model="form.code" class="form-control" :disabled="!!editId" /></div>
      <div class="mb-2"><label class="form-label">名称</label><input v-model="form.name" class="form-control" /></div>
      <div class="mb-2"><label class="form-label">接入类型</label>
        <select v-model="form.access_type" class="form-select"><option value="nvr_agent">nvr_agent</option><option value="rtsp">rtsp</option><option value="gb28181">gb28181</option><option value="push">push</option></select>
      </div>
      <div class="row g-2 mb-2">
        <div class="col"><label class="form-label">NVR Agent ID</label><input v-model="form.nvr_agent_id" class="form-control" /></div>
        <div class="col"><label class="form-label">Agent 摄像头 ID</label><input v-model="form.agent_camera_id" class="form-control" /></div>
      </div>
      <div class="mb-2"><label class="form-label">主码流 URL</label><input v-model="form.main_url" class="form-control" placeholder="可选" /></div>
      <div class="mb-2"><label class="form-label">子码流 URL</label><input v-model="form.sub_url" class="form-control" placeholder="可选" /></div>
      <div class="mb-2"><label class="form-label">备注</label><input v-model="form.remark" class="form-control" /></div>
    </div>
    <template #footer>
      <button class="btn" @click="show = false">取消</button>
      <button class="btn btn-primary" @click="save">保存</button>
    </template>
  </CdModal>

  <CdModal :show="showImport" title="批量导入摄像头" @close="showImport = false">
    <div class="modal-body">
      <div class="text-secondary small mb-2">
        每行一条：<code>code,name,access_type,main_url,sub_url,remark</code>。首行若为表头会自动跳过；含逗号的内容请用双引号包裹。
      </div>
      <textarea v-model="importText" class="form-control mb-2" rows="8" spellcheck="false"></textarea>
      <div v-if="importTally" class="mb-2">
        <span class="badge bg-success me-1">成功 {{ importTally.created }}</span>
        <span class="badge bg-danger me-1">失败 {{ importTally.failed }}</span>
        <span class="badge bg-secondary">共 {{ importTally.total }}</span>
      </div>
      <div v-if="importResult && importResult.length" class="table-responsive" style="max-height: 220px; overflow:auto">
        <table class="table table-sm card-table">
          <thead><tr><th>行</th><th>编号</th><th>结果</th><th>说明</th></tr></thead>
          <tbody>
            <tr v-for="r in importResult" :key="r.index">
              <td>{{ r.index }}</td>
              <td>{{ r.code || "-" }}</td>
              <td><span class="badge" :class="r.ok ? 'bg-success' : 'bg-danger'">{{ r.ok ? "成功" : "失败" }}</span></td>
              <td class="text-secondary">{{ r.error || (r.id ? `#${r.id}` : "") }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <template #footer>
      <button class="btn" @click="showImport = false">关闭</button>
      <button class="btn btn-primary" :disabled="importing" @click="runImport">
        {{ importing ? "导入中…" : "开始导入" }}
      </button>
    </template>
  </CdModal>
</template>
