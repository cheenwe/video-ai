<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { IconSearch, IconRefresh, IconApi } from "@tabler/icons-vue";
import { fmtDate, ApiError } from "@/api/client";
import * as apiCallLogApi from "@/api/apiCallLog";
import type { ApiCallLogRow } from "@/api/apiCallLog";
import { usePageSize } from "@/composables/usePageSize";
import { useToast } from "@/composables/useToast";
import CdPagination from "@/components/CdPagination.vue";
import CdDatePicker from "@/components/CdDatePicker.vue";

const toast = useToast();
const { pageSize, ensurePageSize } = usePageSize();

const items = ref<ApiCallLogRow[]>([]);
const total = ref(0);
const page = ref(1);
const loading = ref(false);

const tokenId = ref("");
const userId = ref("");
const method = ref("");
const statusCode = ref("");
const pathKw = ref("");
const timeStart = ref("");
const timeEnd = ref("");
const keyword = ref("");

// 前端关键字过滤（方法/路径/IP/令牌 ID/用户 ID），作用于当前页已加载数据
const filteredItems = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw) return items.value;
  return items.value.filter((row) =>
    [row.method, row.path, row.ip, String(row.token_id), String(row.user_id)].some(
      (v) => String(v || "").toLowerCase().includes(kw),
    ),
  );
});

const methodOptions = [
  { value: "", label: "全部方法" },
  { value: "GET", label: "GET" },
  { value: "POST", label: "POST" },
  { value: "PUT", label: "PUT" },
  { value: "DELETE", label: "DELETE" },
  { value: "PATCH", label: "PATCH" },
];

const statusOptions = [
  { value: "", label: "全部状态" },
  { value: "2", label: "2xx 成功" },
  { value: "3", label: "3xx 重定向" },
  { value: "4", label: "4xx 客户端错误" },
  { value: "5", label: "5xx 服务端错误" },
];

function statusBadgeClass(code: number) {
  if (code >= 200 && code < 300) return "bg-success-lt";
  if (code >= 300 && code < 400) return "bg-info-lt";
  if (code >= 400 && code < 500) return "bg-warning-lt";
  if (code >= 500) return "bg-danger-lt";
  return "bg-secondary-lt";
}

function methodBadgeClass(m: string) {
  switch (m) {
    case "GET":
      return "bg-success-lt";
    case "POST":
      return "bg-primary-lt";
    case "PUT":
      return "bg-warning-lt";
    case "DELETE":
      return "bg-danger-lt";
    default:
      return "bg-secondary-lt";
  }
}

function fmtDuration(ms: number) {
  if (ms < 1000) return `${ms} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
}

async function load() {
  loading.value = true;
  try {
    const res = await apiCallLogApi.listApiCallLogs({
      page: page.value,
      page_size: pageSize.value,
      token_id: tokenId.value ? Number(tokenId.value) : undefined,
      user_id: userId.value ? Number(userId.value) : undefined,
      method: method.value || undefined,
      status_code: statusCode.value ? Number(statusCode.value) : undefined,
      path: pathKw.value || undefined,
      time_start: timeStart.value ? new Date(timeStart.value).toISOString() : undefined,
      time_end: timeEnd.value ? new Date(timeEnd.value).toISOString() : undefined,
    });
    items.value = res.items;
    total.value = res.total;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

function search() {
  page.value = 1;
  load();
}

function reset() {
  tokenId.value = "";
  userId.value = "";
  method.value = "";
  statusCode.value = "";
  pathKw.value = "";
  timeStart.value = "";
  timeEnd.value = "";
  page.value = 1;
  load();
}

watch(page, () => load());

onMounted(async () => {
  await ensurePageSize();
  load();
});
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h3 class="card-title mb-0 d-inline-flex align-items-center gap-2">
        <IconApi :size="20" :stroke="1.75" /> API 调用日志
      </h3>
      <p class="text-secondary small mt-1 mb-0">记录通过个人访问令牌（PAT）发起的 API 调用明细。</p>
    </div>

    <div class="card-body border-bottom pb-3">
      <div class="d-flex gap-2 mb-2 flex-wrap align-items-center">
        <input
          v-model="keyword"
          class="form-control form-control-sm"
          placeholder="搜索关键字..."
          style="max-width: 14rem"
          @keyup.enter="search"
        />
        <input
          v-model="tokenId"
          type="number"
          class="form-control form-control-sm"
          placeholder="令牌 ID"
          style="max-width: 8rem"
          @keyup.enter="search"
        />
        <input
          v-model="userId"
          type="number"
          class="form-control form-control-sm"
          placeholder="用户 ID"
          style="max-width: 8rem"
          @keyup.enter="search"
        />
        <select
          v-model="method"
          class="form-select form-select-sm"
          style="max-width: 8rem"
          @change="search"
        >
          <option v-for="o in methodOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
        <select
          v-model="statusCode"
          class="form-select form-select-sm"
          style="max-width: 9rem"
          @change="search"
        >
          <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
        <input
          v-model="pathKw"
          class="form-control form-control-sm"
          placeholder="路径关键字 如 /api/storages"
          style="max-width: 14rem"
          @keyup.enter="search"
        />
      </div>
      <div class="d-flex gap-2 flex-wrap align-items-center">
        <div class="d-flex align-items-center gap-1">
          <span class="text-secondary small text-nowrap">开始</span>
          <CdDatePicker
            v-model="timeStart"
            type="datetime"
            placeholder="开始时间"
            size="sm"
            style="width: 13rem"
            @change="search"
          />
        </div>
        <div class="d-flex align-items-center gap-1">
          <span class="text-secondary small text-nowrap">结束</span>
          <CdDatePicker
            v-model="timeEnd"
            type="datetime"
            placeholder="结束时间"
            size="sm"
            style="width: 13rem"
            @change="search"
          />
        </div>
        <button type="button" class="btn btn-sm btn-primary d-inline-flex align-items-center gap-1" @click="search">
          <IconSearch :size="16" :stroke="1.75" /> 查询
        </button>
        <button type="button" class="btn btn-sm btn-ghost-secondary d-inline-flex align-items-center gap-1" @click="reset">
          <IconRefresh :size="16" :stroke="1.75" /> 重置
        </button>
      </div>
    </div>

    <div class="table-responsive">
      <table class="table table-vcenter card-table cd-admin-table mb-0">
        <thead>
          <tr>
            <th style="width: 60px">ID</th>
            <th style="width: 90px">令牌</th>
            <th style="width: 80px">用户</th>
            <th style="width: 80px">方法</th>
            <th>路径</th>
            <th style="width: 90px">状态码</th>
            <th style="width: 100px">耗时</th>
            <th style="width: 140px">IP</th>
            <th style="width: 180px">时间</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="9" class="text-secondary text-center py-4">加载中…</td>
          </tr>
          <tr v-else-if="items.length && !filteredItems.length">
            <td colspan="9" class="text-secondary text-center py-4">未找到匹配的日志</td>
          </tr>
          <tr v-else-if="!items.length">
            <td colspan="9" class="text-secondary text-center py-4">暂无数据</td>
          </tr>
          <tr v-for="row in filteredItems" :key="row.id">
            <td>{{ row.id }}</td>
            <td>{{ row.token_id }}</td>
            <td>{{ row.user_id }}</td>
            <td>
              <span class="badge" :class="methodBadgeClass(row.method)">{{ row.method }}</span>
            </td>
            <td class="text-truncate" style="max-width: 320px" :title="row.path">
              <code class="small">{{ row.path }}</code>
            </td>
            <td>
              <span class="badge" :class="statusBadgeClass(row.status_code)">{{ row.status_code }}</span>
            </td>
            <td>{{ fmtDuration(row.duration_ms) }}</td>
            <td>{{ row.ip }}</td>
            <td>{{ fmtDate(row.created_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="card-footer">
      <CdPagination
        v-model:page="page"
        :total="total"
        :page-size="pageSize"
        :loading="loading"
      />
    </div>
  </div>
</template>
