<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ApiError } from "@/api/client";
import * as api from "@/api/videoAi";
import { useToast } from "@/composables/useToast";

const toast = useToast();
const summary = ref<Record<string, any>>({});
const runtime = ref<any[]>([]);
const checks = ref<{ name: string; ok: boolean; detail: string }[]>([]);

async function load() {
  try {
    const [s, r, sc] = await Promise.all([api.reportSummary(), api.runtimeMetrics(), api.videoSelfCheck()]);
    summary.value = s || {};
    runtime.value = r || [];
    checks.value = sc.checks || [];
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  }
}
onMounted(load);
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <h2 class="page-title">统计报表</h2>
    <div class="text-secondary">{{ summary.note || '事件趋势与运行指标' }}</div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="row row-cards mb-3">
      <div class="col-sm-6 col-lg-3" v-for="k in [
        ['event_total','近7日事件'],['pending','待处理'],['confirmed','已确认'],['false_positive','误报标记'],
        ['running_deployments','运行布控'],['cameras','摄像头'],['published_boundaries','已发布边界'],['active_scene','现场仍活跃'],
      ]" :key="k[0]">
        <div class="card"><div class="card-body">
          <div class="subheader">{{ k[1] }}</div>
          <div class="h1 mb-0">{{ summary[k[0]] ?? '-' }}</div>
        </div></div>
      </div>
    </div>
    <div class="row g-3">
      <div class="col-lg-6">
        <div class="card"><div class="card-header"><h3 class="card-title">平台自检</h3></div>
          <div class="list-group list-group-flush">
            <div v-for="c in checks" :key="c.name" class="list-group-item d-flex justify-content-between">
              <span>{{ c.name }}</span>
              <span :class="c.ok ? 'text-success' : 'text-danger'">{{ c.ok ? 'OK' : 'FAIL' }} · {{ c.detail }}</span>
            </div>
          </div>
        </div>
      </div>
      <div class="col-lg-6">
        <div class="card"><div class="card-header"><h3 class="card-title">运行时指标</h3></div>
          <div class="table-responsive"><table class="table table-vcenter card-table">
            <thead><tr><th>布控</th><th>摄像头</th><th>结果</th><th>FPS</th><th>帧龄ms</th></tr></thead>
            <tbody>
              <tr v-for="r in runtime" :key="r.deployment_id">
                <td>{{ r.deployment_id }}</td><td>{{ r.camera_id }}</td><td>{{ r.last_result }}</td>
                <td>{{ r.actual_fps }}</td><td>{{ r.frame_age_ms }}</td>
              </tr>
            </tbody>
          </table></div>
        </div>
      </div>
    </div>
  </div></div>
</template>
