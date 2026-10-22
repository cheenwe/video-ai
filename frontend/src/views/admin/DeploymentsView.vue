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
const form = ref({ name: "", camera_id: 0, template_id: 0, boundary_version_id: 0, threshold_near: 0.15, schedule_cron: "" });

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
  bounds.value = (await api.listBoundaries(form.value.camera_id)).filter((b) => b.status === "published");
  if (bounds.value.length) form.value.boundary_version_id = bounds.value[0].id;
}

function openCreate() {
  form.value = {
    name: "抓斗越界布控", camera_id: cameras.value[0]?.id || 0,
    template_id: templates.value[0]?.id || 0, boundary_version_id: 0, threshold_near: 0.15, schedule_cron: "",
  };
  show.value = true;
  onCameraChange();
}

async function save() {
  try {
    await api.createDeployment({
      name: form.value.name,
      camera_id: form.value.camera_id,
      template_id: form.value.template_id || null,
      boundary_version_id: form.value.boundary_version_id || null,
      threshold_near: form.value.threshold_near,
      schedule_cron: form.value.schedule_cron || "",
    });
    show.value = false;
    toast.show("已创建");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "创建失败");
  }
}

async function start(d: Deployment) {
  try {
    const check = await api.selfCheckDeployment(d.id);
    if (!check.ok) {
      toast.show("自检未通过，请检查边界与模板");
      return;
    }
    await api.startDeployment(d.id);
    toast.show("已启动（演示推理）");
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

onMounted(load);
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <div class="row align-items-center">
      <div class="col"><h2 class="page-title">布控管理</h2><div class="text-secondary">期望版本与运行时已应用版本分开显示；启动前自检</div></div>
      <div class="col-auto"><button class="btn btn-primary" v-permission="'deployment:edit'" @click="openCreate">新建布控</button></div>
    </div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="card"><div class="table-responsive">
      <table class="table table-vcenter card-table">
        <thead><tr><th>名称</th><th>摄像头</th><th>状态</th><th>期望/已应用</th><th>运行结果</th><th></th></tr></thead>
        <tbody>
          <tr v-for="d in items" :key="d.id">
            <td>{{ d.name }}</td>
            <td>#{{ d.camera_id }}</td>
            <td><span class="badge">{{ d.status }}</span></td>
            <td>{{ d.desired_revision }} / {{ d.applied_revision }}</td>
            <td>{{ d.runtime?.last_result || '-' }} <span class="text-secondary">{{ d.runtime?.last_error }}</span></td>
            <td class="text-end">
              <button class="btn btn-sm btn-success" v-permission="'deployment:control'" @click="start(d)">启动</button>
              <button class="btn btn-sm ms-1" v-permission="'deployment:control'" @click="pause(d)">暂停</button>
              <button class="btn btn-sm btn-outline-danger ms-1" v-permission="'deployment:control'" @click="stop(d)">停止</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div></div>
  </div></div>
  <CdModal :show="show" title="新建布控" @close="show = false">
    <div class="modal-body">
      <div class="mb-2"><label class="form-label">名称</label><input v-model="form.name" class="form-control" /></div>
      <div class="mb-2"><label class="form-label">摄像头</label>
        <select v-model.number="form.camera_id" class="form-select" @change="onCameraChange">
          <option v-for="c in cameras" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
      </div>
      <div class="mb-2"><label class="form-label">业务模板</label>
        <select v-model.number="form.template_id" class="form-select">
          <option v-for="t in templates" :key="t.id" :value="t.id">{{ t.name }} ({{ t.version }})</option>
        </select>
      </div>
      <div class="mb-2"><label class="form-label">已发布边界</label>
        <select v-model.number="form.boundary_version_id" class="form-select">
          <option v-for="b in bounds" :key="b.id" :value="b.id">v{{ b.version }}</option>
        </select>
      </div>
      <div class="mb-2"><label class="form-label">临边阈值</label><input v-model.number="form.threshold_near" type="number" step="0.01" class="form-control" /></div>
      <div class="mb-2"><label class="form-label">时段 Cron（空=始终；如 * 8-18 * * 1-5）</label><input v-model="form.schedule_cron" class="form-control" placeholder="可选" /></div>
    </div>
    <template #footer>
      <button class="btn" @click="show = false">取消</button>
      <button class="btn btn-primary" @click="save">保存</button>
    </template>
  </CdModal>
</template>
