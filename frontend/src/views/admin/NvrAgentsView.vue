<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  IconPlus, IconPencil, IconTrash, IconRefresh, IconPlugConnected, IconDeviceFloppy, IconSearch, IconPlayerPlay,
} from "@tabler/icons-vue";
import { ApiError, fmtDate } from "@/api/client";
import * as nvrApi from "@/api/nvr";
import type { AgentCamera, AgentConfig, AgentNVR, NvrAgent, NvrChannel, DeviceInfo } from "@/api/nvr";
import { useToast } from "@/composables/useToast";
import { useAuthStore } from "@/stores/auth";
import CdModal from "@/components/CdModal.vue";

const toast = useToast();
const auth = useAuthStore();
const errMsg = (e: unknown, fallback: string) => (e instanceof ApiError ? e.message : fallback);

// ---------- agent 节点 ----------
const agents = ref<NvrAgent[]>([]);
const loading = ref(false);
const selectedId = ref<number | null>(null);
const selected = computed(() => agents.value.find((a) => a.id === selectedId.value) || null);
const testing = ref<number | null>(null);

async function loadAgents() {
  loading.value = true;
  try {
    agents.value = (await nvrApi.listAgents()).items;
    if (selectedId.value && !selected.value) selectedId.value = null;
    if (!selectedId.value && agents.value.length) selectAgent(agents.value[0].id);
  } catch (e) {
    toast.show(errMsg(e, "加载失败"));
  } finally {
    loading.value = false;
  }
}

function statusBadge(a: NvrAgent) {
  if (!a.enabled) return { cls: "bg-secondary-lt", text: "已停用" };
  if (a.last_status === "online") return { cls: "bg-green-lt", text: "在线" };
  if (a.last_status === "unauthorized") return { cls: "bg-orange-lt", text: "Token 错误" };
  if (a.last_status === "offline") return { cls: "bg-red-lt", text: "离线" };
  return { cls: "bg-secondary-lt", text: "未检测" };
}

async function testSaved(a: NvrAgent) {
  testing.value = a.id;
  try {
    const r = await nvrApi.testAgent(a.id);
    toast.show(`连接成功：nvr-agent ${r.version}，${r.camera_count} 路摄像头`);
  } catch (e) {
    toast.show(errMsg(e, "连接失败"));
  } finally {
    testing.value = null;
    loadAgents();
  }
}

// 节点表单
const showForm = ref(false);
const editing = ref<NvrAgent | null>(null);
const saving = ref(false);
const draftTesting = ref(false);
const draftResult = ref<{ ok: boolean; text: string } | null>(null);
const form = ref({ name: "", base_url: "http://127.0.0.1:8090", token: "", clearToken: false, enabled: 1, remark: "" });

function openCreate() {
  editing.value = null;
  form.value = { name: "", base_url: "http://127.0.0.1:8090", token: "", clearToken: false, enabled: 1, remark: "" };
  draftResult.value = null;
  showForm.value = true;
}

function openEdit(a: NvrAgent) {
  editing.value = a;
  form.value = { name: a.name, base_url: a.base_url, token: "", clearToken: false, enabled: a.enabled, remark: a.remark };
  draftResult.value = null;
  showForm.value = true;
}

/** 编辑时 Token 留空表示沿用已保存的 Token */
function formToken(): string | undefined {
  if (form.value.clearToken) return "";
  if (form.value.token) return form.value.token;
  return editing.value ? undefined : "";
}

async function testDraft() {
  draftTesting.value = true;
  draftResult.value = null;
  try {
    const token = formToken();
    const r = await nvrApi.testAgentDraft({
      id: editing.value?.id,
      base_url: form.value.base_url,
      token: token === undefined ? null : token,
    });
    draftResult.value = { ok: true, text: `连接成功：nvr-agent ${r.version}，已配置 ${r.camera_count} 路摄像头` };
  } catch (e) {
    draftResult.value = { ok: false, text: errMsg(e, "连接失败") };
  } finally {
    draftTesting.value = false;
  }
}

async function saveAgent() {
  if (!form.value.name.trim() || !form.value.base_url.trim()) {
    toast.show("名称和地址不能为空");
    return;
  }
  saving.value = true;
  try {
    const payload = {
      name: form.value.name,
      base_url: form.value.base_url,
      enabled: form.value.enabled,
      remark: form.value.remark,
      token: formToken(),
    };
    if (editing.value) {
      await nvrApi.updateAgent(editing.value.id, payload);
      toast.show("保存成功");
    } else {
      const created = await nvrApi.createAgent(payload);
      selectedId.value = created.id;
      toast.show("创建成功");
      await nvrApi.testAgent(created.id).catch(() => undefined);
    }
    showForm.value = false;
    await loadAgents();
    if (selectedId.value) loadConfig();
  } catch (e) {
    toast.show(errMsg(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}

async function removeAgent(a: NvrAgent) {
  if (!confirm(`确定删除 nvr-agent「${a.name}」吗？只删除平台登记，不影响该电脑上的录像文件。`)) return;
  try {
    await nvrApi.deleteAgent(a.id);
    toast.show("已删除");
    if (selectedId.value === a.id) {
      selectedId.value = null;
      cfg.value = null;
    }
    loadAgents();
  } catch (e) {
    toast.show(errMsg(e, "删除失败"));
  }
}

// ---------- agent 设备配置 ----------
const canConfig = computed(() => auth.can("nvr:config"));
const cfg = ref<AgentConfig | null>(null);
const cfgLoading = ref(false);
const cfgError = ref("");
const cfgSaving = ref(false);
const cfgMessage = ref<{ ok: boolean; text: string } | null>(null);

function selectAgent(id: number) {
  selectedId.value = id;
  loadConfig();
}

function normalizeConfig(c: AgentConfig): AgentConfig {
  c.nvrs = c.nvrs || [];
  c.cameras = (c.cameras || []).map((cam) => ({
    ...cam,
    record: Object.assign(
      { enabled: false, stream: "main", segment_minutes: 10, retention_days: 7, audio: false },
      cam.record as Partial<AgentCamera["record"]> | undefined,
    ),
  }));
  return c;
}

async function loadConfig() {
  cfg.value = null;
  cfgError.value = "";
  cfgMessage.value = null;
  discovery.value = {};
  probeResult.value = {};
  if (!selectedId.value || !canConfig.value) return;
  cfgLoading.value = true;
  try {
    cfg.value = normalizeConfig(await nvrApi.getAgentConfig(selectedId.value));
  } catch (e) {
    cfgError.value = errMsg(e, "读取配置失败");
  } finally {
    cfgLoading.value = false;
  }
}

function uniqueID(prefix: string, used: string[]) {
  for (let i = 1; ; i++) if (!used.includes(prefix + i)) return prefix + i;
}

function addNVR() {
  if (!cfg.value) return;
  cfg.value.nvrs.push({
    id: uniqueID("nvr", cfg.value.nvrs.map((n) => n.id)), name: "", host: "", http_port: 80, rtsp_port: 554,
    https: false, insecure_tls: false, username: "admin", password: "", password_env: "",
  });
}

function removeNVR(i: number) {
  if (!cfg.value) return;
  const id = cfg.value.nvrs[i].id;
  const used = cfg.value.cameras.filter((c) => c.nvr === id).length;
  if (used && !confirm(`有 ${used} 路摄像头使用该 NVR，删除后需要同时调整这些摄像头。继续？`)) return;
  cfg.value.nvrs.splice(i, 1);
}

/** 修改 NVR ID 时同步引用它的摄像头 */
function renameNVR(n: AgentNVR, ev: Event) {
  if (!cfg.value) return;
  const next = (ev.target as HTMLInputElement).value.trim();
  const prev = n.id;
  n.id = next;
  if (prev && next && prev !== next) cfg.value.cameras.forEach((c) => { if (c.nvr === prev) c.nvr = next; });
}

function newCamera(partial: Partial<AgentCamera>): AgentCamera {
  return {
    id: "", name: "", nvr: "", channel: 1, rtsp_path: "", url: "", live_stream: "sub", live_transcode: false,
    record: { enabled: false, stream: "main", segment_minutes: 10, retention_days: 7, audio: false },
    ...partial,
  };
}

function addCamera() {
  if (!cfg.value) return;
  cfg.value.cameras.push(newCamera({
    id: uniqueID("cam", cfg.value.cameras.map((c) => c.id)),
    nvr: cfg.value.nvrs[0]?.id || "",
  }));
}

// 通道发现
const discovery = ref<Record<number, { loading: boolean; error?: string; device?: DeviceInfo; channels?: NvrChannel[] }>>({});

async function discover(i: number) {
  if (!cfg.value || !selectedId.value) return;
  discovery.value[i] = { loading: true };
  try {
    const r = await nvrApi.debugNVR(selectedId.value, cfg.value.nvrs[i]);
    discovery.value[i] = { loading: false, device: r.device, channels: r.channels || [] };
  } catch (e) {
    discovery.value[i] = { loading: false, error: errMsg(e, "连接失败") };
  }
}

function hasChannel(n: AgentNVR, ch: number) {
  return !!cfg.value?.cameras.some((c) => c.nvr === n.id && Number(c.channel) === ch && !c.url);
}

function addChannel(n: AgentNVR, ch: NvrChannel) {
  if (!cfg.value || hasChannel(n, ch.id)) return;
  let id = `${n.id}_ch${ch.id}`;
  const used = cfg.value.cameras.map((c) => c.id);
  if (used.includes(id)) id = uniqueID(id + "_", used);
  cfg.value.cameras.push(newCamera({ id, name: ch.name || `通道${ch.id}`, nvr: n.id, channel: ch.id }));
  toast.show(`已添加通道 ${ch.id}，保存配置后生效`);
}

// 码流测试
const probeResult = ref<Record<string, { loading: boolean; ok?: boolean; text?: string }>>({});

async function probe(c: AgentCamera, stream: "main" | "sub") {
  if (!cfg.value || !selectedId.value) return;
  const key = `${c.id}:${stream}`;
  const n = cfg.value.nvrs.find((x) => x.id === c.nvr);
  if (!n) {
    probeResult.value[key] = { loading: false, ok: false, text: "请先选择 NVR" };
    return;
  }
  probeResult.value[key] = { loading: true };
  try {
    const r = await nvrApi.debugStream(selectedId.value, { nvr: n, channel: Number(c.channel), stream, rtsp_path: c.rtsp_path });
    probeResult.value[key] = { loading: false, ok: true, text: r.result };
  } catch (e) {
    probeResult.value[key] = { loading: false, ok: false, text: errMsg(e, "测试失败") };
  }
}

async function saveConfig() {
  if (!cfg.value || !selectedId.value) return;
  cfgSaving.value = true;
  cfgMessage.value = null;
  try {
    const payload: AgentConfig = {
      ...cfg.value,
      cameras: cfg.value.cameras.map((c) => ({ ...c, channel: Number(c.channel) || 0 })),
    };
    const r = await nvrApi.saveAgentConfig(selectedId.value, payload);
    cfgMessage.value = r.restart_required
      ? { ok: true, text: "已保存并生效。监听地址已修改，需要在该电脑上重新运行 nvr-agent，并在节点中更新地址。" }
      : { ok: true, text: "已保存并立即生效。" };
    toast.show("配置已保存");
    const keep = cfgMessage.value;
    await loadConfig();
    cfgMessage.value = keep;
    loadAgents();
  } catch (e) {
    cfgMessage.value = { ok: false, text: errMsg(e, "保存失败") };
  } finally {
    cfgSaving.value = false;
  }
}

onMounted(loadAgents);
</script>

<template>
  <div>
    <div class="d-flex align-items-center justify-content-between gap-2 mb-4">
      <div>
        <h2>NVR 接入</h2>
        <p class="text-secondary mb-0">登记运行 nvr-agent 的电脑，在线配置海康 NVR、发现通道并测试码流</p>
      </div>
    </div>

    <div class="row g-4">
      <div class="col-xl-4">
        <div class="card">
          <div class="card-header d-flex align-items-center justify-content-between gap-2">
            <h3 class="card-title mb-0">nvr-agent 节点</h3>
            <div class="d-flex gap-2">
              <button class="btn btn-sm btn-ghost-secondary" title="刷新" @click="loadAgents"><IconRefresh :size="15" /></button>
              <button class="btn btn-primary btn-sm" v-permission="'nvr:create'" @click="openCreate">
                <IconPlus :size="16" class="me-1" />登记
              </button>
            </div>
          </div>
          <div class="card-body">
            <div v-if="loading && !agents.length" class="text-secondary text-center py-3">加载中…</div>
            <div v-else-if="!agents.length" class="text-secondary py-3">
              <p class="mb-2">尚未登记 nvr-agent。</p>
              <ul class="small ps-3 mb-0">
                <li>同机部署：地址填 <code>http://127.0.0.1:8090</code>，Token 可留空。</li>
                <li>分机部署：agent 监听 <code>0.0.0.0:8090</code> 并设置 Token，这里填该电脑的局域网地址和同一个 Token。</li>
              </ul>
            </div>
            <div v-else class="list-group list-group-flush">
              <button
                v-for="a in agents"
                :key="a.id"
                class="list-group-item list-group-item-action d-flex align-items-center justify-content-between"
                :class="{ active: selectedId === a.id }"
                @click="selectAgent(a.id)"
              >
                <div class="text-start" style="min-width: 0">
                  <div class="fw-semibold">{{ a.name }}</div>
                  <div class="small text-secondary text-truncate">{{ a.base_url }}</div>
                  <div class="small mt-1">
                    <span class="badge" :class="statusBadge(a).cls">{{ statusBadge(a).text }}</span>
                    <span v-if="a.version" class="text-secondary ms-2">v{{ a.version }}</span>
                    <span v-if="a.last_seen_at" class="text-secondary ms-2">{{ fmtDate(a.last_seen_at) }}</span>
                  </div>
                  <div v-if="a.last_error && a.last_status !== 'online'" class="small text-danger mt-1 text-wrap">{{ a.last_error }}</div>
                </div>
                <div class="d-flex gap-1" @click.stop>
                  <button class="btn btn-sm btn-icon" title="测试连接" :disabled="testing === a.id" @click="testSaved(a)">
                    <IconPlugConnected :size="15" />
                  </button>
                  <button class="btn btn-sm btn-icon" title="编辑" v-permission="'nvr:update'" @click="openEdit(a)">
                    <IconPencil :size="15" />
                  </button>
                  <button class="btn btn-sm btn-icon text-danger" title="删除" v-permission="'nvr:delete'" @click="removeAgent(a)">
                    <IconTrash :size="15" />
                  </button>
                </div>
              </button>
            </div>
          </div>
        </div>
      </div>

      <div class="col-xl-8">
        <div v-if="!selected" class="card"><div class="card-body text-secondary text-center py-5">请先登记并选择一个 nvr-agent 节点</div></div>
        <div v-else-if="!canConfig" class="card"><div class="card-body text-secondary text-center py-5">没有「NVR 设备配置与联调」权限</div></div>
        <template v-else>
          <div v-if="cfgLoading" class="card"><div class="card-body text-secondary text-center py-5">正在读取 {{ selected.name }} 的配置…</div></div>
          <div v-else-if="cfgError" class="card">
            <div class="card-body">
              <div class="alert alert-danger mb-3">{{ cfgError }}</div>
              <button class="btn btn-sm" @click="loadConfig"><IconRefresh :size="15" class="me-1" />重试</button>
            </div>
          </div>
          <template v-else-if="cfg">
            <div class="card mb-3">
              <div class="card-header"><h3 class="card-title mb-0">程序设置 · {{ selected.name }}</h3></div>
              <div class="card-body">
                <div class="row g-3">
                  <div class="col-md-6">
                    <label class="form-label">监听地址</label>
                    <input v-model.trim="cfg.listen" class="form-control" placeholder="127.0.0.1:8090" />
                    <div class="form-hint">同机部署用 127.0.0.1:8090；分机部署用 0.0.0.0:8090（必须设置 Token）。修改后需重新运行 agent。</div>
                  </div>
                  <div class="col-md-6">
                    <label class="form-label">API Token</label>
                    <input v-model="cfg.api_token" type="password" class="form-control" autocomplete="new-password" />
                    <div class="form-hint">显示 ******** 表示保持不变；修改后平台会自动同步保存的 Token。</div>
                  </div>
                  <div class="col-md-4">
                    <label class="form-label">FFmpeg 路径</label>
                    <input v-model.trim="cfg.ffmpeg" class="form-control" placeholder="ffmpeg.exe" />
                  </div>
                  <div class="col-md-4">
                    <label class="form-label">FFprobe 路径</label>
                    <input v-model.trim="cfg.ffprobe" class="form-control" placeholder="ffprobe.exe" />
                  </div>
                  <div class="col-md-4">
                    <label class="form-label">数据目录</label>
                    <input v-model.trim="cfg.data_dir" class="form-control" placeholder="data" />
                  </div>
                </div>
              </div>
            </div>

            <div class="card mb-3">
              <div class="card-header d-flex align-items-center justify-content-between">
                <h3 class="card-title mb-0">NVR 设备</h3>
                <button class="btn btn-sm" @click="addNVR"><IconPlus :size="15" class="me-1" />添加 NVR</button>
              </div>
              <div class="card-body">
                <div v-if="!cfg.nvrs.length" class="text-secondary">尚未添加 NVR</div>
                <div v-for="(n, i) in cfg.nvrs" :key="i" class="border rounded p-3 mb-3">
                  <div class="d-flex justify-content-between align-items-center mb-2">
                    <strong>{{ n.name || n.id || "新 NVR" }}</strong>
                    <button class="btn btn-sm btn-ghost-danger" @click="removeNVR(i)"><IconTrash :size="15" /></button>
                  </div>
                  <div class="row g-2">
                    <div class="col-md-3"><label class="form-label">设备 ID</label><input :value="n.id" class="form-control form-control-sm" @change="renameNVR(n, $event)" /></div>
                    <div class="col-md-3"><label class="form-label">名称</label><input v-model.trim="n.name" class="form-control form-control-sm" placeholder="1号NVR" /></div>
                    <div class="col-md-3"><label class="form-label">IP / 主机名</label><input v-model.trim="n.host" class="form-control form-control-sm" placeholder="192.168.1.64" /></div>
                    <div class="col-md-3"><label class="form-label">用户名</label><input v-model.trim="n.username" class="form-control form-control-sm" /></div>
                    <div class="col-md-3"><label class="form-label">密码</label><input v-model="n.password" type="password" class="form-control form-control-sm" autocomplete="new-password" /></div>
                    <div class="col-md-2"><label class="form-label">HTTP 端口</label><input v-model.number="n.http_port" type="number" min="1" class="form-control form-control-sm" /></div>
                    <div class="col-md-2"><label class="form-label">RTSP 端口</label><input v-model.number="n.rtsp_port" type="number" min="1" class="form-control form-control-sm" /></div>
                    <div class="col-md-5 d-flex align-items-end gap-3">
                      <label class="form-check mb-1"><input v-model="n.https" type="checkbox" class="form-check-input" /><span class="form-check-label">HTTPS</span></label>
                      <label class="form-check mb-1"><input v-model="n.insecure_tls" type="checkbox" class="form-check-input" /><span class="form-check-label">允许自签名证书</span></label>
                    </div>
                  </div>
                  <div class="mt-2 d-flex align-items-center gap-2">
                    <button class="btn btn-sm btn-outline-primary" :disabled="discovery[i]?.loading" @click="discover(i)">
                      <IconSearch :size="15" class="me-1" />{{ discovery[i]?.loading ? "连接中…" : "连接并发现通道" }}
                    </button>
                    <span class="small text-secondary">由 nvr-agent 所在电脑访问 NVR；未保存的修改也可直接测试</span>
                  </div>
                  <div v-if="discovery[i] && !discovery[i].loading" class="mt-2">
                    <div v-if="discovery[i].error" class="alert alert-danger py-2 mb-0">{{ discovery[i].error }}</div>
                    <div v-else class="bg-body-tertiary rounded p-2 small">
                      <div class="mb-1">
                        <strong>{{ discovery[i].device?.model || discovery[i].device?.device_name }}</strong>
                        <span class="text-secondary ms-2">固件 {{ discovery[i].device?.firmware_version }}</span>
                        <span class="text-secondary ms-2">序列号 {{ discovery[i].device?.serial_number }}</span>
                      </div>
                      <div v-if="!discovery[i].channels?.length" class="text-secondary">未发现 IP 通道</div>
                      <table v-else class="table table-sm mb-0">
                        <tbody>
                          <tr v-for="ch in discovery[i].channels" :key="ch.id">
                            <td>通道 {{ ch.id }}</td>
                            <td>{{ ch.name }}</td>
                            <td class="text-secondary">{{ ch.ip }}</td>
                            <td>
                              <span v-if="ch.online === undefined || ch.online === null" class="badge bg-secondary-lt">未知</span>
                              <span v-else class="badge" :class="ch.online ? 'bg-green-lt' : 'bg-red-lt'">{{ ch.online ? "在线" : "离线" }}</span>
                            </td>
                            <td class="text-end">
                              <button class="btn btn-sm" :disabled="hasChannel(n, ch.id)" @click="addChannel(n, ch)">
                                {{ hasChannel(n, ch.id) ? "已添加" : "添加为摄像头" }}
                              </button>
                            </td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div class="card mb-3">
              <div class="card-header d-flex align-items-center justify-content-between">
                <h3 class="card-title mb-0">摄像头</h3>
                <button class="btn btn-sm" @click="addCamera"><IconPlus :size="15" class="me-1" />添加摄像头</button>
              </div>
              <div class="card-body">
                <div v-if="!cfg.cameras.length" class="text-secondary">尚未添加摄像头，可在上方 NVR「连接并发现通道」后一键添加</div>
                <div v-for="(c, i) in cfg.cameras" :key="i" class="border rounded p-3 mb-3">
                  <div class="d-flex justify-content-between align-items-center mb-2">
                    <strong>{{ c.name || c.id || "新摄像头" }}</strong>
                    <button class="btn btn-sm btn-ghost-danger" @click="cfg.cameras.splice(i, 1)"><IconTrash :size="15" /></button>
                  </div>
                  <div class="row g-2">
                    <div class="col-md-3"><label class="form-label">摄像头 ID</label><input v-model.trim="c.id" class="form-control form-control-sm" /></div>
                    <div class="col-md-3"><label class="form-label">名称</label><input v-model.trim="c.name" class="form-control form-control-sm" /></div>
                    <div class="col-md-3">
                      <label class="form-label">NVR</label>
                      <select v-model="c.nvr" class="form-select form-select-sm">
                        <option value="">（不使用 NVR）</option>
                        <option v-for="n in cfg.nvrs" :key="n.id" :value="n.id">{{ n.name || n.id }}</option>
                      </select>
                    </div>
                    <div class="col-md-3"><label class="form-label">通道号</label><input v-model.number="c.channel" type="number" min="1" class="form-control form-control-sm" /></div>
                    <div class="col-md-3">
                      <label class="form-label">预览码流</label>
                      <select v-model="c.live_stream" class="form-select form-select-sm"><option value="sub">子码流</option><option value="main">主码流</option></select>
                    </div>
                    <div class="col-md-3">
                      <label class="form-label">录制码流</label>
                      <select v-model="c.record.stream" class="form-select form-select-sm"><option value="main">主码流</option><option value="sub">子码流</option></select>
                    </div>
                    <div class="col-md-3"><label class="form-label">分片（分钟）</label><input v-model.number="c.record.segment_minutes" type="number" min="1" class="form-control form-control-sm" /></div>
                    <div class="col-md-3"><label class="form-label">保留（天）</label><input v-model.number="c.record.retention_days" type="number" min="1" class="form-control form-control-sm" /></div>
                    <div class="col-md-6"><label class="form-label">自定义 RTSP 地址（可选，填写后不走 NVR）</label><input v-model.trim="c.url" class="form-control form-control-sm" placeholder="rtsp://..." /></div>
                    <div class="col-md-6"><label class="form-label">自定义路径（可选）</label><input v-model.trim="c.rtsp_path" class="form-control form-control-sm" placeholder="/Streaming/Channels/33{stream}" /></div>
                    <div class="col-12 d-flex flex-wrap gap-3">
                      <label class="form-check"><input v-model="c.live_transcode" type="checkbox" class="form-check-input" /><span class="form-check-label">预览时 H.265 转 H.264</span></label>
                      <label class="form-check"><input v-model="c.record.enabled" type="checkbox" class="form-check-input" /><span class="form-check-label">自动录制</span></label>
                      <label class="form-check"><input v-model="c.record.audio" type="checkbox" class="form-check-input" /><span class="form-check-label">录制音频</span></label>
                    </div>
                  </div>
                  <div class="mt-2 d-flex gap-2">
                    <button class="btn btn-sm btn-outline-primary" :disabled="!!probeResult[c.id + ':main']?.loading || !c.nvr" @click="probe(c, 'main')">
                      <IconPlayerPlay :size="15" class="me-1" />测试主码流
                    </button>
                    <button class="btn btn-sm btn-outline-primary" :disabled="!!probeResult[c.id + ':sub']?.loading || !c.nvr" @click="probe(c, 'sub')">
                      <IconPlayerPlay :size="15" class="me-1" />测试子码流
                    </button>
                  </div>
                  <template v-for="s in ['main', 'sub']" :key="s">
                    <pre
                      v-if="probeResult[c.id + ':' + s] && !probeResult[c.id + ':' + s].loading"
                      class="small mt-2 mb-0 p-2 rounded text-wrap"
                      :class="probeResult[c.id + ':' + s].ok ? 'bg-body-tertiary' : 'bg-red-lt'"
                    >{{ s === 'main' ? '主码流' : '子码流' }}：{{ probeResult[c.id + ':' + s].text }}</pre>
                  </template>
                </div>
              </div>
            </div>

            <div class="card">
              <div class="card-body d-flex align-items-center gap-3 flex-wrap">
                <button class="btn btn-primary" :disabled="cfgSaving" @click="saveConfig">
                  <IconDeviceFloppy :size="16" class="me-1" />{{ cfgSaving ? "保存中…" : "保存配置" }}
                </button>
                <button class="btn" :disabled="cfgSaving" @click="loadConfig">放弃修改</button>
                <span v-if="cfgMessage" :class="cfgMessage.ok ? 'text-success' : 'text-danger'">{{ cfgMessage.text }}</span>
              </div>
            </div>
          </template>
        </template>
      </div>
    </div>

    <CdModal :show="showForm" :title="editing ? '编辑 nvr-agent' : '登记 nvr-agent'" size="lg" @close="showForm = false">
      <div class="modal-body">
        <div class="mb-3">
          <label class="form-label">名称 <span class="text-danger">*</span></label>
          <input v-model="form.name" class="form-control" placeholder="如 一号机房录像机" />
        </div>
        <div class="mb-3">
          <label class="form-label">地址 <span class="text-danger">*</span></label>
          <input v-model.trim="form.base_url" class="form-control" placeholder="http://127.0.0.1:8090" />
          <div class="form-hint">同机部署填 http://127.0.0.1:8090；分机部署填 agent 电脑的局域网地址，如 http://192.168.1.20:8090。</div>
        </div>
        <div class="mb-3">
          <label class="form-label">API Token</label>
          <input v-model="form.token" type="password" class="form-control" autocomplete="new-password" :disabled="form.clearToken"
            :placeholder="editing ? (editing.token_set ? '已设置，留空则不修改' : '未设置') : '与 agent 配置中的 api_token 一致'" />
          <label v-if="editing?.token_set" class="form-check mt-2">
            <input v-model="form.clearToken" type="checkbox" class="form-check-input" />
            <span class="form-check-label">清除已保存的 Token</span>
          </label>
          <div class="form-hint">Token 加密保存在平台，浏览器不会直接访问 agent。</div>
        </div>
        <div class="row g-3">
          <div class="col-4">
            <label class="form-label">状态</label>
            <select v-model.number="form.enabled" class="form-select">
              <option :value="1">启用</option>
              <option :value="0">停用</option>
            </select>
          </div>
          <div class="col-8">
            <label class="form-label">备注</label>
            <input v-model="form.remark" class="form-control" />
          </div>
        </div>
        <div v-if="draftResult" class="alert mt-3 mb-0" :class="draftResult.ok ? 'alert-success' : 'alert-danger'">{{ draftResult.text }}</div>
      </div>
      <div class="modal-footer">
        <button class="btn me-auto" :disabled="draftTesting" @click="testDraft">
          <IconPlugConnected :size="16" class="me-1" />{{ draftTesting ? "测试中…" : "测试连接" }}
        </button>
        <button class="btn" @click="showForm = false">取消</button>
        <button class="btn btn-primary" :disabled="saving" @click="saveAgent">{{ saving ? "保存中…" : "保存" }}</button>
      </div>
    </CdModal>
  </div>
</template>
