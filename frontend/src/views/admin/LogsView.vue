<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import {
  IconSearch,
  IconRefresh,
  IconListDetails,
  IconLogin2,
  IconShieldCheck,
  IconRepeat,
  IconClock,
  IconEye,
  IconDownload,
} from "@tabler/icons-vue";
import { fmtDate, ApiError, getToken } from "@/api/client";
import * as logsApi from "@/api/sysLogs";
import type { LogCategory, SysLogRow } from "@/api/sysLogs";
import { usePageSize } from "@/composables/usePageSize";
import { useToast } from "@/composables/useToast";
import CdPagination from "@/components/CdPagination.vue";
import CdModal from "@/components/CdModal.vue";
import { logActionLabel } from "@/utils/logActionLabel";
import type { SysLogDetail } from "@/api/sysLogs";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const toast = useToast();
const { pageSize, ensurePageSize } = usePageSize();

const tabs: {
  key: LogCategory;
  label: string;
  icon: typeof IconListDetails;
}[] = [
  { key: "log", label: "操作日志", icon: IconListDetails },
  { key: "login", label: "登录日志", icon: IconLogin2 },
  { key: "audit", label: "审计日志", icon: IconShieldCheck },
];

const items = ref<SysLogRow[]>([]);
const total = ref(0);
const page = ref(1);
const loading = ref(false);
const keyword = ref("");
const result = ref("");
const source = ref("");

const sourceOptions = [
  { value: "", label: "全部来源" },
  { value: "web", label: "Web" },
  { value: "api", label: "API" },
];

const activeTab = computed<LogCategory>(() => {
  const t = route.query.tab;
  if (t === "login" || t === "audit") return t;
  return "log";
});

const showTypeColumn = computed(() => activeTab.value === "log");

const subTagOptions = [
  { value: "", label: "全部子类型" },
  { value: "operation", label: "配置操作" },
  { value: "department", label: "部门" },
];

const subTag = ref("");

const resultOptions = [
  { value: "", label: "全部结果" },
  { value: "ok", label: "成功" },
  { value: "fail", label: "失败" },
];

function tagLabel(v: string) {
  const map: Record<string, string> = {
    login: "登录",
    operation: "操作",
    audit: "审计",
    sync: "同步",
    file: "文件",
    storage: "存储",
    share: "分享",
    agent: "智能体",
    department: "部门",
    plugin: "插件",
    site: "站点",
  };
  return map[v] || v || "—";
}

function tagClass(v: string) {
  if (v === "login") return "cd-log-tag--login";
  if (v === "sync") return "cd-log-tag--sync";
  if (v === "file") return "cd-log-tag--file";
  if (v === "storage") return "cd-log-tag--storage";
  if (v === "share") return "cd-log-tag--share";
  if (v === "audit") return "cd-log-tag--audit";
  if (v === "operation") return "cd-log-tag--default";
  return "cd-log-tag--default";
}

function resultLabel(v: string) {
  if (v === "ok" || v === "success") return "成功";
  if (v === "fail" || v === "failed") return "失败";
  return v || "—";
}

function actionLabel(action: string) {
  const labels: Record<string,string> = { "settings.general": "更新基本设置", "settings.branding": "更新平台外观", "settings.security": "更新安全策略" }; return labels[action] || logActionLabel(action, t);
}

function isResultOk(v: string) {
  return v === "ok" || v === "success";
}

function userInitial(name?: string) {
  const n = (name || "?").trim();
  return n ? n.slice(0, 1).toUpperCase() : "?";
}

async function load() {
  loading.value = true;
  try {
    const res = await logsApi.listSysLogs({
      page: page.value,
      page_size: pageSize.value,
      keyword: keyword.value || undefined,
      category: activeTab.value,
      tag: activeTab.value === "log" && subTag.value ? subTag.value : undefined,
      result: result.value || undefined,
      source:
        activeTab.value === "log" && source.value ? source.value : undefined,
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

// ---------- 详情弹窗 ----------
const showDetail = ref(false);
const detailLoading = ref(false);
const detail = ref<SysLogDetail | null>(null);

async function viewDetail(l: SysLogRow) {
  showDetail.value = true;
  detailLoading.value = true;
  detail.value = null;
  try {
    detail.value = await logsApi.getSysLog(l.id);
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    detailLoading.value = false;
  }
}

function prettyJSON(v: unknown): string {
  if (v == null) return "";
  try {
    return JSON.stringify(v, null, 2);
  } catch {
    return String(v);
  }
}

// ---------- 导出 ----------
function doExport() {
  const url = logsApi.exportSysLogs({
    keyword: keyword.value || undefined,
    tag: activeTab.value === "log" && subTag.value ? subTag.value : undefined,
    result: result.value || undefined,
  });
  // 携带认证头下载
  const token = getToken();
  fetch(url, { headers: { Authorization: `Bearer ${token}` } })
    .then((res) => res.blob())
    .then((blob) => {
      const link = document.createElement("a");
      link.href = URL.createObjectURL(blob);
      link.download = `audit-logs-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, "")}.csv`;
      link.click();
      URL.revokeObjectURL(link.href);
    })
    .catch(() => toast.show("导出失败"));
}

function switchTab(tab: LogCategory) {
  if (tab === activeTab.value) return;
  subTag.value = "";
  source.value = "";
  page.value = 1;
  router.replace({ path: "/admin/logs", query: { tab } });
}

watch(
  () => route.query.tab,
  () => {
    page.value = 1;
    subTag.value = "";
    source.value = "";
    load();
  },
);

watch(page, () => load());

onMounted(async () => {
  if (!route.query.tab) {
    await router.replace({ path: "/admin/logs", query: { tab: "log" } });
  }
  await ensurePageSize();
  load();
});
</script>

<template>
  <div class="card cd-settings-layout">
    <aside class="cd-settings-nav">
      <nav class="cd-settings-nav-list">
        <button
          v-for="t in tabs"
          :key="t.key"
          type="button"
          class="cd-settings-nav-item"
          :class="{ active: activeTab === t.key }"
          @click="switchTab(t.key)"
        >
          <component :is="t.icon" :size="17" :stroke="1.75" />
          <span>{{ t.label }}</span>
        </button>
      </nav>
    </aside>

    <main class="cd-settings-main cd-logs-main">
      <div class="cd-logs-toolbar">
        <div class="cd-logs-toolbar-filters">
          <div class="cd-logs-field">
            <div class="input-group input-group-sm">
              <span class="input-group-text"><IconSearch :size="14" /></span>
              <input
                v-model="keyword"
                class="form-control"
                placeholder="搜索用户、动作、备注…"
                @keyup.enter="search"
              />
            </div>
          </div>
          <select
            v-if="showTypeColumn"
            v-model="subTag"
            class="form-select form-select-sm cd-logs-select"
            @change="search"
          >
            <option v-for="o in subTagOptions" :key="o.value" :value="o.value">
              {{ o.label }}
            </option>
          </select>
          <select
            v-model="result"
            class="form-select form-select-sm cd-logs-select"
            @change="search"
          >
            <option v-for="o in resultOptions" :key="o.value" :value="o.value">
              {{ o.label }}
            </option>
          </select>
          <select
            v-if="showTypeColumn"
            v-model="source"
            class="form-select form-select-sm cd-logs-select"
            @change="search"
          >
            <option v-for="o in sourceOptions" :key="o.value" :value="o.value">
              {{ o.label }}
            </option>
          </select>
        </div>
        <div class="cd-logs-toolbar-actions">
          <button type="button" class="btn btn-sm btn-primary" @click="search">
            查询
          </button>
          <button
            type="button"
            class="btn btn-sm btn-outline-secondary"
            title="导出 CSV"
            @click="doExport"
           v-permission="['log:export']">
            <IconDownload :size="15" class="me-1" />导出
          </button>
          <button
            type="button"
            class="btn btn-sm btn-ghost-secondary"
            :disabled="loading"
            title="刷新"
            @click="load"
          >
            <IconRefresh :size="16" />
          </button>
        </div>
      </div>

      <div class="cd-logs-scroll table-responsive">
        <table class="table table-vcenter card-table cd-admin-table mb-0">
          <thead>
            <tr>
              <th class="cd-logs-col-time">时间</th>
              <th>用户</th>
              <th v-if="showTypeColumn">类型</th>
              <th v-if="showTypeColumn">来源</th>
              <th>结果</th>
              <th>动作</th>
              <th>IP</th>
              <th>备注</th>
              <th style="width: 80px">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td
                :colspan="showTypeColumn ? 9 : 7"
                class="text-secondary text-center py-4"
              >
                加载中…
              </td>
            </tr>
            <tr v-else-if="!items.length">
              <td
                :colspan="showTypeColumn ? 9 : 7"
                class="text-secondary text-center py-4"
              >
                暂无匹配的日志记录
              </td>
            </tr>
            <tr v-for="l in items" :key="l.id">
              <td class="cd-logs-col-time">
                <span class="cd-logs-time">{{ fmtDate(l.created_at) }}</span>
              </td>
              <td>
                <div class="cd-logs-user">
                  <span class="cd-logs-avatar">{{
                    userInitial(l.username)
                  }}</span>
                  <span class="cd-logs-username">{{ l.username || "—" }}</span>
                </div>
              </td>
              <td v-if="showTypeColumn">
                <span class="cd-log-tag" :class="tagClass(l.tag)">{{
                  tagLabel(l.tag)
                }}</span>
              </td>
              <td v-if="showTypeColumn">
                <span class="cd-logs-source">{{ l.source_name || "—" }}</span>
              </td>
              <td>
                <span
                  class="cd-log-result"
                  :class="
                    isResultOk(l.result)
                      ? 'cd-log-result--ok'
                      : 'cd-log-result--fail'
                  "
                >
                  <span class="cd-log-result-dot" />
                  {{ resultLabel(l.result) }}
                </span>
              </td>
              <td>
                <span class="cd-logs-action" :title="l.action">{{
                  actionLabel(l.action)
                }}</span>
              </td>
              <td>
                <span class="cd-logs-ip">{{ l.ip || "—" }}</span>
              </td>
              <td>
                <span class="cd-logs-remark" :title="l.remark">{{
                  l.remark || "—"
                }}</span>
              </td>
              <td>
                <button
                  type="button"
                  class="btn btn-sm btn-icon"
                  title="查看详情"
                  @click="viewDetail(l)"
                >
                  <IconEye :size="15" />
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="cd-logs-footer card-footer">
        <CdPagination
          v-model:page="page"
          :total="total"
          :page-size="pageSize"
          :loading="loading"
        />
      </div>
    </main>

    <!-- 审计详情弹窗 -->
    <CdModal :show="showDetail" title="审计详情" size="lg" @close="showDetail = false">
      <div class="modal-body">
        <div v-if="detailLoading" class="text-secondary text-center py-4">加载中…</div>
        <div v-else-if="!detail" class="text-secondary text-center py-4">无数据</div>
        <div v-else>
          <div class="row g-2 mb-3 small">
            <div class="col-6"><span class="text-secondary">时间：</span>{{ fmtDate(detail.created_at) }}</div>
            <div class="col-6"><span class="text-secondary">用户：</span>{{ detail.username || "—" }}</div>
            <div class="col-6"><span class="text-secondary">IP：</span>{{ detail.ip || "—" }}</div>
            <div class="col-6"><span class="text-secondary">标签：</span>{{ tagLabel(detail.tag) }}</div>
            <div class="col-6"><span class="text-secondary">动作：</span>{{ actionLabel(detail.action) }}</div>
            <div class="col-6"><span class="text-secondary">结果：</span>{{ resultLabel(detail.result) }}</div>
            <div class="col-12" v-if="detail.remark"><span class="text-secondary">备注：</span>{{ detail.remark }}</div>
          </div>
          <div v-if="detail.before || detail.after">
            <div class="d-flex gap-3">
              <div v-if="detail.before" class="flex-1">
                <div class="text-secondary small mb-1">变更前</div>
                <pre class="bg-light p-2 rounded small overflow-auto" style="max-height: 280px">{{ prettyJSON(detail.before) }}</pre>
              </div>
              <div v-if="detail.after" class="flex-1">
                <div class="text-secondary small mb-1">变更后</div>
                <pre class="bg-light p-2 rounded small overflow-auto" style="max-height: 280px">{{ prettyJSON(detail.after) }}</pre>
              </div>
            </div>
          </div>
          <div v-else class="text-secondary text-center py-3">该操作无变更数据</div>
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn" @click="showDetail = false">关闭</button>
      </div>
    </CdModal>
  </div>
</template>
