<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ApiError } from "@/api/client";
import * as api from "@/api/videoAi";
import { evidenceURL } from "@/api/videoAi";
import type { VideoEvent } from "@/api/videoAi";
import { useToast } from "@/composables/useToast";
import CdModal from "@/components/CdModal.vue";

const toast = useToast();
const items = ref<VideoEvent[]>([]);
const total = ref(0);
const handleStatus = ref("");
const detail = ref<any>(null);
const showClose = ref(false);
const closeId = ref(0);
const conclusion = ref("real");
const remark = ref("");

async function load() {
  try {
    const params: Record<string, string | number> = { page: 1, page_size: 50 };
    if (handleStatus.value) params.handle_status = handleStatus.value;
    const res = await api.listEvents(params);
    items.value = res.items || [];
    total.value = res.total || 0;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  }
}

async function openDetail(id: number) {
  try { detail.value = await api.getEvent(id); }
  catch (e) { toast.show(e instanceof ApiError ? e.message : "加载详情失败"); }
}

async function confirm(ev: VideoEvent) {
  try { await api.actEvent(ev.id, { action: "confirm" }); toast.show("已确认"); await load(); }
  catch (e) { toast.show(e instanceof ApiError ? e.message : "失败"); }
}

function openClose(ev: VideoEvent) {
  closeId.value = ev.id; conclusion.value = "real"; remark.value = ""; showClose.value = true;
}

async function doClose() {
  try {
    await api.actEvent(closeId.value, { action: "close", conclusion: conclusion.value, remark: remark.value });
    showClose.value = false; toast.show("已关闭"); await load();
  } catch (e) { toast.show(e instanceof ApiError ? e.message : "关闭失败"); }
}

async function batch() {
  const ids = items.value.filter((e) => e.handle_status === "pending").map((e) => e.id);
  if (!ids.length) return;
  try { await api.batchConfirmEvents(ids); toast.show("批量确认完成"); await load(); }
  catch (e) { toast.show(e instanceof ApiError ? e.message : "失败"); }
}

onMounted(load);
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <div class="row align-items-center">
      <div class="col"><h2 class="page-title">告警中心</h2><div class="text-secondary">现场状态与人工处理状态分离；关闭不改变算法结论</div></div>
      <div class="col-auto"><button class="btn" v-permission="'event:handle'" @click="batch">批量确认待处理</button></div>
    </div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="card mb-3"><div class="card-body row g-2">
      <div class="col-auto"><select v-model="handleStatus" class="form-select" @change="load">
        <option value="">全部处理状态</option>
        <option value="pending">pending</option><option value="confirmed">confirmed</option>
        <option value="processing">processing</option><option value="closed">closed</option>
      </select></div>
      <div class="col-auto"><button class="btn" @click="load">刷新</button></div>
    </div></div>
    <div class="card"><div class="table-responsive">
      <table class="table table-vcenter card-table">
        <thead><tr><th>ID</th><th>类别</th><th>级别</th><th>现场</th><th>处理</th><th>开始</th><th></th></tr></thead>
        <tbody>
          <tr v-for="e in items" :key="e.id">
            <td>{{ e.id }}</td><td>{{ e.category }}</td><td>{{ e.level }}</td>
            <td>{{ e.scene_status }}</td><td>{{ e.handle_status }}</td><td>{{ e.started_at }}</td>
            <td class="text-end">
              <button class="btn btn-sm" @click="openDetail(e.id)">详情</button>
              <button class="btn btn-sm ms-1" v-permission="'event:handle'" @click="confirm(e)">确认</button>
              <button class="btn btn-sm ms-1" v-permission="'event:handle'" @click="openClose(e)">关闭</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div><div class="card-footer text-secondary">共 {{ total }} 条</div></div>
    <div v-if="detail" class="card mt-3"><div class="card-body">
      <h3 class="card-title">事件 #{{ detail.event.id }}</h3>
      <div v-if="detail.evidences?.length" class="mb-3">
        <div class="subheader mb-1">证据</div>
        <a v-for="evd in detail.evidences" :key="evd.id" class="btn btn-sm me-1" :href="evidenceURL(detail.event.id, evd.id)" target="_blank" rel="noopener">
          {{ evd.kind }} · {{ evd.status }}
        </a>
      </div>
      <pre class="small">{{ JSON.stringify(detail, null, 2) }}</pre>
    </div></div>
  </div></div>
  <CdModal :show="showClose" title="关闭事件" @close="showClose = false">
    <div class="modal-body">
      <div class="mb-2"><label class="form-label">结论</label>
        <select v-model="conclusion" class="form-select">
          <option value="real">真实事件</option><option value="false_positive">误报</option><option value="unknown">无法确认</option>
        </select>
      </div>
      <div class="mb-2"><label class="form-label">说明</label><textarea v-model="remark" class="form-control" rows="3" /></div>
    </div>
    <template #footer>
      <button class="btn" @click="showClose = false">取消</button>
      <button class="btn btn-primary" @click="doClose">关闭</button>
    </template>
  </CdModal>
</template>
