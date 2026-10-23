<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ApiError } from "@/api/client";
import * as api from "@/api/videoAi";
import type { BoundaryVersion, Camera } from "@/api/videoAi";
import { useToast } from "@/composables/useToast";

const toast = useToast();
const cameras = ref<Camera[]>([]);
const cameraId = ref(0);
const items = ref<BoundaryVersion[]>([]);
const pointsText = ref("100,400\n640,420\n1180,400");

async function loadCams() {
  const res = await api.listCameras({ page: 1, page_size: 100 });
  cameras.value = res.items || [];
  if (!cameraId.value && cameras.value.length) cameraId.value = cameras.value[0].id;
}

async function load() {
  if (!cameraId.value) return;
  try {
    items.value = await api.listBoundaries(cameraId.value);
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  }
}

async function candidate() {
  // Automatic candidates need a real model running on live frames; the demo
  // polyline generator was removed. The API fails honestly — surface why.
  try {
    await api.boundaryCandidate(cameraId.value);
    toast.show("已获取候选");
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "获取候选失败：需要真实模型推理与实时视频帧，请手动绘制");
  }
}

async function createDraft() {
  const points = pointsText.value.trim().split(/\n+/).map((line) => {
    const [x, y] = line.split(",").map(Number);
    return { x, y };
  });
  try {
    await api.createBoundary({
      camera_id: cameraId.value,
      name: "draft",
      ref_width: 1280,
      ref_height: 720,
      geometry: { kind: "polyline", pit_side: "below", buffer_norm: 0.02, points },
    });
    toast.show("已创建草稿");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  }
}

async function publish(id: number) {
  try {
    await api.publishBoundary(id);
    toast.show("已发布");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "发布失败");
  }
}

async function rollback(id: number) {
  try {
    await api.rollbackBoundary(id);
    toast.show("已生成回滚草稿");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "回滚失败");
  }
}

onMounted(async () => { await loadCams(); await load(); });
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <h2 class="page-title">坑边管理</h2>
    <div class="text-secondary">边界版本：草稿 / 已发布 / 失效 / 归档。发布前校验点数与自交。</div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="row g-3">
      <div class="col-lg-5">
        <div class="card">
          <div class="card-body">
            <label class="form-label">摄像头</label>
            <select v-model.number="cameraId" class="form-select mb-2" @change="load">
              <option v-for="c in cameras" :key="c.id" :value="c.id">{{ c.name }}</option>
            </select>
            <label class="form-label">折线点（x,y 每行一个，参考 1280×720）</label>
            <textarea v-model="pointsText" class="form-control mb-2" rows="6" />
            <div class="btn-list">
              <button class="btn" v-permission="'boundary:edit'" @click="candidate"
                title="需要真实模型推理（ONNX worker）与实时视频帧；当前不可用时会明确提示">AI 候选</button>
              <button class="btn btn-primary" v-permission="'boundary:edit'" @click="createDraft">保存草稿</button>
            </div>
          </div>
        </div>
      </div>
      <div class="col-lg-7">
        <div class="card">
          <div class="table-responsive">
            <table class="table table-vcenter card-table">
              <thead><tr><th>版本</th><th>状态</th><th>来源</th><th></th></tr></thead>
              <tbody>
                <tr v-for="b in items" :key="b.id">
                  <td>v{{ b.version }} {{ b.name }}</td>
                  <td><span class="badge">{{ b.status }}</span></td>
                  <td>{{ b.source }}</td>
                  <td class="text-end">
                    <button class="btn btn-sm" v-permission="'boundary:publish'" :disabled="b.status==='published'" @click="publish(b.id)">发布</button>
                    <button class="btn btn-sm ms-1" v-permission="'boundary:edit'" @click="rollback(b.id)">回滚</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </div></div>
</template>
