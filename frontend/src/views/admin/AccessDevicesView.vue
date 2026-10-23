<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ApiError } from "@/api/client";
import * as api from "@/api/videoAi";
import type { AccessDevice } from "@/api/videoAi";
import { useToast } from "@/composables/useToast";
import CdModal from "@/components/CdModal.vue";

const toast = useToast();
const items = ref<AccessDevice[]>([]);
const show = ref(false);
const form = ref({ kind: "gb28181", code: "", name: "", config: "{}" });

async function load() {
  try { items.value = await api.listAccessDevices(); }
  catch (e) { toast.show(e instanceof ApiError ? e.message : "加载失败"); }
}

async function save() {
  try {
    let cfg = {};
    try { cfg = JSON.parse(form.value.config || "{}"); } catch { toast.show("配置 JSON 无效"); return; }
    await api.createAccessDevice({ kind: form.value.kind, code: form.value.code, name: form.value.name, config: cfg });
    show.value = false; toast.show("已登记"); await load();
  } catch (e) { toast.show(e instanceof ApiError ? e.message : "保存失败"); }
}

const probing = ref<Record<number, boolean>>({});
const syncing = ref<Record<number, boolean>>({});
const lastProbe = ref<Record<number, { ok: boolean; message: string; next_steps?: string[] }>>({});

async function probe(d: AccessDevice) {
  probing.value[d.id] = true;
  try {
    const res = await api.probeAccessDevice(d.id);
    lastProbe.value[d.id] = { ok: res.ok, message: res.message, next_steps: res.next_steps };
    toast.show(res.ok ? `可达：${res.message}` : `不可达：${res.message}`);
    await load();
  } catch (e) { toast.show(e instanceof ApiError ? e.message : "探测失败"); }
  finally { probing.value[d.id] = false; }
}

async function syncCamera(d: AccessDevice) {
  syncing.value[d.id] = true;
  try {
    const res = await api.syncAccessDeviceCamera(d.id);
    toast.show(res.created ? `已创建平台摄像头 #${res.camera.id}` : `已更新平台摄像头 #${res.camera.id}`);
    await load();
  } catch (e) { toast.show(e instanceof ApiError ? e.message : "同步失败"); }
  finally { syncing.value[d.id] = false; }
}

function statusClass(s: string) {
  if (s === "reachable") return "bg-success";
  if (s === "unreachable") return "bg-danger";
  return "bg-secondary";
}

async function remove(d: AccessDevice) {
  if (!confirm(`删除 ${d.code}？`)) return;
  try { await api.deleteAccessDevice(d.id); await load(); }
  catch (e) { toast.show(e instanceof ApiError ? e.message : "删除失败"); }
}

onMounted(load);
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <div class="row align-items-center">
      <div class="col">
        <h2 class="page-title">扩展接入</h2>
        <div class="text-secondary">GB28181 / RTMP / HTTP-FLV / HLS 输入骨架；真机信令需型号联调</div>
      </div>
      <div class="col-auto"><button class="btn btn-primary" v-permission="'access:edit'" @click="show=true">登记设备</button></div>
    </div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="card"><div class="table-responsive">
      <table class="table table-vcenter card-table">
        <thead><tr><th>类型</th><th>编码</th><th>名称</th><th>状态</th><th>错误</th><th></th></tr></thead>
        <tbody>
          <tr v-for="d in items" :key="d.id">
            <td>{{ d.kind }}</td><td>{{ d.code }}</td><td>{{ d.name }}</td>
            <td><span class="badge" :class="statusClass(d.status)">{{ d.status }}</span></td>
            <td class="text-secondary">{{ d.last_error }}</td>
            <td class="text-end">
              <button class="btn btn-sm" :disabled="probing[d.id]" v-permission="'access:edit'" @click="probe(d)">
                {{ probing[d.id] ? "探测中…" : "探测" }}
              </button>
              <button class="btn btn-sm ms-1" :disabled="syncing[d.id] || d.status !== 'reachable'"
                :title="d.status === 'reachable' ? '同步为平台摄像头' : '需先探测成功'"
                v-permission="'access:edit'" @click="syncCamera(d)">
                {{ syncing[d.id] ? "同步中…" : "同步摄像头" }}
              </button>
              <span v-if="d.camera_id" class="badge bg-info text-dark ms-1">摄像头 #{{ d.camera_id }}</span>
              <button class="btn btn-sm btn-outline-danger ms-1" v-permission="'access:edit'" @click="remove(d)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div></div>
    <div v-if="Object.keys(lastProbe).length" class="card mt-3">
      <div class="card-header"><h3 class="card-title">最近探测结果</h3></div>
      <div class="list-group list-group-flush">
        <div v-for="(p, id) in lastProbe" :key="id" class="list-group-item">
          <div class="d-flex justify-content-between">
            <span class="fw-bold small">设备 #{{ id }}</span>
            <span class="badge" :class="p.ok ? 'bg-success' : 'bg-danger'">{{ p.ok ? "可达" : "不可达" }}</span>
          </div>
          <div class="small text-secondary">{{ p.message }}</div>
          <ul v-if="p.next_steps && p.next_steps.length" class="small text-secondary mb-0 mt-1">
            <li v-for="(s, i) in p.next_steps" :key="i">{{ s }}</li>
          </ul>
        </div>
      </div>
    </div>
  </div></div>
  <CdModal :show="show" title="登记扩展接入" @close="show = false">
    <div class="modal-body">
      <div class="mb-2"><label class="form-label">类型</label>
        <select v-model="form.kind" class="form-select">
          <option value="gb28181">gb28181</option><option value="rtmp_push">rtmp_push</option>
          <option value="http_flv">http_flv</option><option value="hls_in">hls_in</option>
        </select>
      </div>
      <div class="mb-2"><label class="form-label">编码</label><input v-model="form.code" class="form-control" /></div>
      <div class="mb-2"><label class="form-label">名称</label><input v-model="form.name" class="form-control" /></div>
      <div class="mb-2"><label class="form-label">配置 JSON</label><textarea v-model="form.config" class="form-control" rows="4" /></div>
    </div>
    <template #footer>
      <button class="btn" @click="show = false">取消</button>
      <button class="btn btn-primary" @click="save">保存</button>
    </template>
  </CdModal>
</template>
