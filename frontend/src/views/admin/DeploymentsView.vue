<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ApiError } from "@/api/client";
import * as api from "@/api/videoAi";
import type { BoundaryVersion, BusinessTemplate, Camera, Deployment } from "@/api/videoAi";
import { useToast } from "@/composables/useToast";
import CdModal from "@/components/CdModal.vue";

const toast = useToast();
const items = ref<Deployment[]>([]);
const cameras = ref<Camera[]>([]);
const templates = ref<BusinessTemplate[]>([]);
const bounds = ref<BoundaryVersion[]>([]);
const show = ref(false);
const editId = ref<number | null>(null);
const saving = ref(false);

const defaultForm = () => ({
  name: "", camera_id: 0, template_id: 0, boundary_version_id: 0,
  analysis_fps: 2, threshold_near: 0.15, threshold_cross: 0,
  trigger_hold_sec: 1, recover_hold_sec: 2,
  schedule_cron: "", mute_notify: 0, osd_enabled: 1,
});
const form = ref(defaultForm());

async function load() {
  try {
    const [d, c, t] = await Promise.all([api.listDeployments(), api.listCameras({ page_size: 100 }), api.listTemplates()]);
    items.value = d || [];
    cameras.value = c.items || [];
    templates.value = t || [];
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  }
}

async function onCameraChange() {
  if (!form.value.camera_id) return;
  bounds.value = (await api.listBoundaries(form.value.camera_id)).filter((b) => b.status === "published");
  form.value.boundary_version_id = bounds.value[0]?.id || 0;
}

function openCreate() {
  editId.value = null;
  form.value = defaultForm();
  form.value.camera_id = cameras.value[0]?.id || 0;
  form.value.template_id = templates.value[0]?.id || 0;
  show.value = true;
  onCameraChange();
}

function openEdit(d: Deployment) {
  editId.value = d.id;
  form.value = {
    name: d.name, camera_id: d.camera_id, template_id: d.template_id || 0,
    boundary_version_id: d.boundary_version_id || 0,
    analysis_fps: d.analysis_fps, threshold_near: d.threshold_near, threshold_cross: d.threshold_cross,
    trigger_hold_sec: d.trigger_hold_sec, recover_hold_sec: d.recover_hold_sec,
    schedule_cron: d.schedule_cron || "", mute_notify: d.mute_notify ? 1 : 0, osd_enabled: d.osd_enabled ? 1 : 0,
  };
  show.value = true;
  onCameraChange().then(() => {
    if (d.boundary_version_id) form.value.boundary_version_id = d.boundary_version_id;
  });
}

function payload() {
  return {
    name: form.value.name,
    camera_id: form.value.camera_id,
    template_id: form.value.template_id || null,
    boundary_version_id: form.value.boundary_version_id || null,
    analysis_fps: form.value.analysis_fps,
    threshold_near: form.value.threshold_near,
    threshold_cross: form.value.threshold_cross,
    trigger_hold_sec: form.value.trigger_hold_sec,
    recover_hold_sec: form.value.recover_hold_sec,
    schedule_cron: form.value.schedule_cron || "",
    mute_notify: form.value.mute_notify,
    osd_enabled: form.value.osd_enabled,
  };
}

async function save() {
  if (!form.value.name.trim() || !form.value.camera_id) {
    toast.show("请填写名称并选择摄像头");
    return;
  }
  saving.value = true;
  try {
    if (editId.value) {
      await api.updateDeployment(editId.value, payload());
      toast.show("已保存，修订号已递增；运行中布控将在下一拍应用");
    } else {
      await api.createDeployment(payload());
      toast.show("已创建");
    }
    show.value = false;
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

async function start(d: Deployment) {
  try {
    const check = await api.selfCheckDeployment(d.id);
    if (!check.ok) {
      toast.show("自检未通过，请检查边界、模板与模型版本");
      return;
    }
    await api.startDeployment(d.id);
    toast.show("已启动");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "启动失败");
  }
}

async function stop(d: Deployment) {
  try { await api.stopDeployment(d.id); toast.show("已停止"); await load(); }
  catch (e) { toast.show(e instanceof ApiError ? e.message : "停止失败"); }
}

async function pause(d: Deployment) {
  try { await api.pauseDeployment(d.id); toast.show("已暂停"); await load(); }
  catch (e) { toast.show(e instanceof ApiError ? e.message : "暂停失败"); }
}

const resultBadge: Record<string, string> = {
  normal: "bg-success", near: "bg-warning text-dark", cross: "bg-danger",
  unknown: "bg-secondary", offline: "bg-dark",
};

onMounted(load);
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <div class="row align-items-center">
      <div class="col"><h2 class="page-title">布控管理</h2><div class="text-secondary">期望版本与运行时已应用版本分开显示；按模板加载模型版本；临边/越界支持滞回判定</div></div>
      <div class="col-auto"><button class="btn btn-primary" v-permission="'deployment:edit'" @click="openCreate">新建布控</button></div>
    </div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="card"><div class="table-responsive">
      <table class="table table-vcenter card-table">
        <thead><tr><th>名称</th><th>摄像头</th><th>状态</th><th>期望/已应用</th><th>分析 FPS</th><th>运行结果</th><th></th></tr></thead>
        <tbody>
          <tr v-for="d in items" :key="d.id">
            <td>{{ d.name }}</td>
            <td>#{{ d.camera_id }}</td>
            <td><span class="badge" :class="d.status === 'running' ? 'bg-success' : d.status === 'fault' ? 'bg-danger' : 'bg-secondary'">{{ d.status }}</span></td>
            <td>{{ d.desired_revision }} / {{ d.applied_revision }}</td>
            <td>{{ d.analysis_fps }}</td>
            <td>
              <span class="badge" :class="resultBadge[d.runtime?.last_result || ''] || 'bg-secondary'">{{ d.runtime?.last_result || '-' }}</span>
              <span class="text-secondary ms-1">{{ d.runtime?.last_error }}</span>
              <span v-if="d.runtime?.backend" class="text-secondary ms-1">({{ d.runtime.backend }}<span v-if="!d.runtime?.model_loaded" class="text-danger">·模型未加载</span>)</span>
            </td>
            <td class="text-end">
              <button class="btn btn-sm" v-permission="'deployment:edit'" @click="openEdit(d)">编辑</button>
              <button class="btn btn-sm btn-success ms-1" v-permission="'deployment:control'" @click="start(d)">启动</button>
              <button class="btn btn-sm ms-1" v-permission="'deployment:control'" @click="pause(d)">暂停</button>
              <button class="btn btn-sm btn-outline-danger ms-1" v-permission="'deployment:control'" @click="stop(d)">停止</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div></div>
  </div></div>
  <CdModal :show="show" :title="editId ? '编辑布控' : '新建布控'" @close="show = false">
    <div class="modal-body">
      <div class="mb-2"><label class="form-label">名称</label><input v-model="form.name" class="form-control" /></div>
      <div class="mb-2"><label class="form-label">摄像头</label>
        <select v-model.number="form.camera_id" class="form-select" :disabled="!!editId" @change="onCameraChange">
          <option v-for="c in cameras" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
      </div>
      <div class="mb-2"><label class="form-label">业务模板（决定加载的模型版本）</label>
        <select v-model.number="form.template_id" class="form-select">
          <option v-for="t in templates" :key="t.id" :value="t.id">{{ t.name }} ({{ t.version }})</option>
        </select>
      </div>
      <div class="mb-2"><label class="form-label">已发布边界</label>
        <select v-model.number="form.boundary_version_id" class="form-select">
          <option v-for="b in bounds" :key="b.id" :value="b.id">v{{ b.version }}</option>
        </select>
      </div>
      <div class="row g-2 mb-2">
        <div class="col"><label class="form-label">分析 FPS</label><input v-model.number="form.analysis_fps" type="number" step="0.5" min="0.1" max="10" class="form-control" /></div>
        <div class="col"><label class="form-label">临边阈值</label><input v-model.number="form.threshold_near" type="number" step="0.01" class="form-control" /></div>
        <div class="col"><label class="form-label">越界阈值（&gt;0 覆盖）</label><input v-model.number="form.threshold_cross" type="number" step="0.01" class="form-control" /></div>
      </div>
      <div class="row g-2 mb-2">
        <div class="col"><label class="form-label">触发保持 (秒)</label><input v-model.number="form.trigger_hold_sec" type="number" step="0.5" min="0" class="form-control" /></div>
        <div class="col"><label class="form-label">恢复保持 (秒)</label><input v-model.number="form.recover_hold_sec" type="number" step="0.5" min="0" class="form-control" /></div>
      </div>
      <div class="mb-2"><label class="form-label">时段 Cron（空=始终；如 * 8-18 * * 1-5）</label><input v-model="form.schedule_cron" class="form-control" placeholder="可选" /></div>
      <div class="row g-2">
        <div class="col-auto form-check form-switch ms-2">
          <input id="mute-notify" v-model="form.mute_notify" type="checkbox" class="form-check-input" role="switch" />
          <label class="form-check-label" for="mute-notify">静音通知（仍记录事件）</label>
        </div>
        <div class="col-auto form-check form-switch ms-2">
          <input id="osd" v-model="form.osd_enabled" type="checkbox" class="form-check-input" role="switch" />
          <label class="form-check-label" for="osd">OSD 叠加</label>
        </div>
      </div>
    </div>
    <template #footer>
      <button class="btn" @click="show = false">取消</button>
      <button class="btn btn-primary" :disabled="saving" @click="save">保存</button>
    </template>
  </CdModal>
</template>
