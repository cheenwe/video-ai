<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { IconDotsVertical } from "@tabler/icons-vue";
import { ApiError } from "@/api/client";
import * as projectsApi from "@/api/projects";
import type { Project, ProjectMember } from "@/api/projects";
import { memberRoleLabel, memberRoleOptions } from "@/api/projects";
import * as cfApi from "@/api/customFields";
import type { CustomFieldWithValue } from "@/api/customFields";
import { usePageSize } from "@/composables/usePageSize";
import { useToast } from "@/composables/useToast";
import CdModal from "@/components/CdModal.vue";
import CdPagination from "@/components/CdPagination.vue";

const toast = useToast();
const { pageSize, ensurePageSize } = usePageSize();

const projects = ref<Project[]>([]);
const total = ref(0);
const page = ref(1);
const loading = ref(false);
const keyword = ref("");

// ---------- 自定义字段（project 模块） ----------
const CUSTOM_FIELD_MODULE = "project";
const cfDefs = ref<CustomFieldWithValue[]>([]);
const cfCreateValues = ref<Record<number, string | string[]>>({});
const cfEditValues = ref<Record<number, string | string[]>>({});
// 列表批量值：cfListValues[projectId][fieldId] = raw string
const cfListValues = ref<Record<number, Record<number, string>>>({});

async function loadCfDefs() {
  try {
    const res = await cfApi.getCustomFieldDefinitions(CUSTOM_FIELD_MODULE);
    cfDefs.value = res.fields || [];
  } catch {
    cfDefs.value = [];
  }
}

function fillDefaultCfValues(): Record<number, string | string[]> {
  const next: Record<number, string | string[]> = {};
  for (const d of cfDefs.value) {
    next[d.id] = d.field_type === "multi_select" ? [] : "";
  }
  return next;
}

async function loadCfValues(projectId: number) {
  if (!cfDefs.value.length) {
    await loadCfDefs();
  }
  cfEditValues.value = {};
  if (!cfDefs.value.length || !projectId) return;
  try {
    const res = await cfApi.getCustomFieldValues(CUSTOM_FIELD_MODULE, projectId);
    const byId = new Map<number, CustomFieldWithValue>();
    for (const f of res.fields || []) byId.set(f.id, f);
    cfDefs.value = cfDefs.value.map((d) => byId.get(d.id) || d);
    const next: Record<number, string | string[]> = {};
    for (const d of cfDefs.value) {
      next[d.id] = cfApi.decodeFieldValue(byId.get(d.id)?.value || "", d.field_type);
    }
    cfEditValues.value = next;
  } catch {
    cfEditValues.value = fillDefaultCfValues();
  }
}

// 列表加载后批量拉取自定义字段值，合并到 cfListValues 供列表列渲染。
async function loadCustomFieldBatchValues() {
  if (!cfDefs.value.length) {
    await loadCfDefs();
  }
  if (!cfDefs.value.length || !projects.value.length) {
    cfListValues.value = {};
    return;
  }
  const ids = projects.value.map((p) => p.id);
  try {
    const res = await cfApi.getCustomFieldBatchValues(CUSTOM_FIELD_MODULE, ids);
    const next: Record<number, Record<number, string>> = {};
    const values = res.values || {};
    for (const eid of Object.keys(values)) {
      const arr = values[eid] || [];
      const rowMap: Record<number, string> = {};
      for (const v of arr) {
        rowMap[v.field_id] = v.value;
      }
      next[Number(eid)] = rowMap;
    }
    cfListValues.value = next;
  } catch {
    cfListValues.value = {};
  }
}

// 列表单元格格式化：把某项目某字段的 raw 值格式化为可读文本。
function cfCellText(projectId: number, fieldId: number): string {
  const def = cfDefs.value.find((d) => d.id === fieldId);
  if (!def) return "—";
  const raw = cfListValues.value[projectId]?.[fieldId] || "";
  return cfApi.formatFieldValue(raw, def.field_type, def.options);
}

function serializeCfValues(values: Record<number, string | string[]>): { field_id: number; value: string }[] {
  return cfDefs.value.map((d) => ({
    field_id: d.id,
    value: cfApi.encodeFieldValue(values[d.id] ?? (d.field_type === "multi_select" ? [] : ""), d.field_type),
  }));
}

function validateCfRequired(values: Record<number, string | string[]>): boolean {
  for (const d of cfDefs.value) {
    if (!d.required) continue;
    const v = values[d.id];
    const empty = Array.isArray(v) ? v.length === 0 : !String(v || "").trim();
    if (empty) {
      toast.show(`请填写「${d.field_name}」`);
      return false;
    }
  }
  return true;
}

async function saveCfAfterCreate(projectId: number) {
  if (!cfDefs.value.length) return;
  const payload = serializeCfValues(cfCreateValues.value);
  if (!payload.length) return;
  try {
    await cfApi.saveCustomFieldValues(CUSTOM_FIELD_MODULE, projectId, payload);
  } catch (e) {
    toast.show(e instanceof ApiError ? `扩展字段保存失败：${e.message}` : "扩展字段保存失败");
  }
}

async function saveCfAfterEdit(projectId: number) {
  if (!cfDefs.value.length) return;
  const payload = serializeCfValues(cfEditValues.value);
  if (!payload.length) return;
  try {
    await cfApi.saveCustomFieldValues(CUSTOM_FIELD_MODULE, projectId, payload);
  } catch (e) {
    toast.show(e instanceof ApiError ? `扩展字段保存失败：${e.message}` : "扩展字段保存失败");
  }
}

// ---------- 新建/编辑项目 ----------
const showEdit = ref(false);
const editing = ref<Project | null>(null);
const form = ref({ code: "", name: "", description: "" });
const formError = ref("");

async function openCreate() {
  editing.value = null;
  form.value = { code: "", name: "", description: "" };
  formError.value = "";
  showEdit.value = true;
  if (!cfDefs.value.length) {
    await loadCfDefs();
  }
  cfCreateValues.value = fillDefaultCfValues();
}

async function openEdit(p: Project) {
  editing.value = p;
  form.value = {
    code: p.code,
    name: p.name,
    description: p.description || "",
  };
  formError.value = "";
  showEdit.value = true;
  await loadCfValues(p.id);
}

async function saveProject() {
  formError.value = "";
  if (!form.value.name.trim()) {
    formError.value = "名称不能为空";
    return;
  }
  if (!editing.value && !form.value.code.trim()) {
    formError.value = "编码不能为空";
    return;
  }
  // 自定义字段必填校验
  const cfValues = editing.value ? cfEditValues.value : cfCreateValues.value;
  if (!validateCfRequired(cfValues)) {
    return;
  }
  try {
    if (editing.value) {
      await projectsApi.updateProject(editing.value.id, {
        name: form.value.name,
        description: form.value.description,
      });
      await saveCfAfterEdit(editing.value.id);
      toast.show("已更新");
    } else {
      const created = await projectsApi.createProject({
        code: form.value.code,
        name: form.value.name,
        description: form.value.description,
      });
      await saveCfAfterCreate(created.id);
      toast.show("已创建");
    }
    showEdit.value = false;
    await loadProjects();
  } catch (e) {
    formError.value = e instanceof ApiError ? e.message : "保存失败";
  }
}

// ---------- 删除 ----------
const showDelete = ref(false);
const deleting = ref<Project | null>(null);

function openDelete(p: Project) {
  deleting.value = p;
  showDelete.value = true;
}

async function confirmDelete() {
  if (!deleting.value) return;
  try {
    await projectsApi.deleteProject(deleting.value.id);
    toast.show("已删除");
    showDelete.value = false;
    await loadProjects();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "删除失败");
  }
}

// ---------- 成员管理 ----------
const showMembers = ref(false);
const memberProject = ref<Project | null>(null);
const members = ref<ProjectMember[]>([]);
const memberLoading = ref(false);

const showAddMember = ref(false);
const addMemberForm = ref({ user_id: "", role: "member" });

async function openMembers(p: Project) {
  memberProject.value = p;
  members.value = [];
  memberLoading.value = true;
  showMembers.value = true;
  try {
    const res = await projectsApi.listProjectMembers(p.id);
    members.value = res.items;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载成员失败");
  } finally {
    memberLoading.value = false;
  }
}

async function addMember() {
  if (!memberProject.value) return;
  const uid = Number(addMemberForm.value.user_id);
  if (!uid || uid < 1) {
    toast.show("请输入有效的用户 ID");
    return;
  }
  try {
    await projectsApi.addProjectMember(memberProject.value.id, {
      user_id: uid,
      role: addMemberForm.value.role,
    });
    toast.show("已添加");
    showAddMember.value = false;
    addMemberForm.value = { user_id: "", role: "member" };
    await openMembers(memberProject.value);
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "添加失败");
  }
}

async function changeMemberRole(m: ProjectMember, role: string) {
  if (!memberProject.value) return;
  try {
    await projectsApi.updateProjectMember(m.id, role);
    toast.show("角色已更新");
    await openMembers(memberProject.value);
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "更新失败");
  }
}

function onRoleChange(m: ProjectMember, e: Event) {
  const target = e.target as HTMLSelectElement;
  changeMemberRole(m, target.value);
}

async function removeMember(m: ProjectMember) {
  if (!memberProject.value) return;
  if (!confirm(`确定移除成员「${m.real_name || m.username}」吗？`)) return;
  try {
    await projectsApi.removeProjectMember(m.id);
    toast.show("已移除");
    await openMembers(memberProject.value);
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "移除失败");
  }
}

// ---------- 加载 ----------
async function loadProjects() {
  loading.value = true;
  try {
    const res = await projectsApi.listProjects({
      page: page.value,
      page_size: pageSize.value,
      keyword: keyword.value || undefined,
    });
    projects.value = res.items;
    total.value = res.total;
    loadCustomFieldBatchValues();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

function search() {
  page.value = 1;
  loadProjects();
}

watch(page, () => loadProjects());

onMounted(async () => {
  await ensurePageSize();
  loadCfDefs();
  loadProjects();
});
</script>

<template>
  <div>
    <div class="d-flex gap-2 mb-3 flex-wrap align-items-center">
      <input
        v-model="keyword"
        class="form-control form-control-sm"
        placeholder="搜索项目名称/编码/描述"
        style="max-width: 14rem"
        @keyup.enter="search"
      />

      <button type="button" class="btn btn-sm btn-primary" @click="openCreate" v-permission="['project:create']">
        新建项目
      </button>
      <button type="button" class="btn btn-sm" @click="loadProjects">刷新</button>
    </div>

    <div class="card">
      <div class="table-responsive">
        <table class="table table-vcenter card-table cd-admin-table">
          <thead>
            <tr>
              <th class="w-1">ID</th>
              <th>项目名称</th>
              <th>编码</th>
              <th>负责人</th>
              <th>状态</th>
              <th v-for="f in cfDefs" :key="f.id">{{ f.field_name }}</th>
              <th class="w-1"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td :colspan="6 + cfDefs.length" class="text-secondary text-center py-4">
                加载中…
              </td>
            </tr>
            <tr v-else-if="!projects.length">
              <td :colspan="6 + cfDefs.length" class="text-secondary text-center py-4">
                暂无项目
              </td>
            </tr>
            <tr v-for="p in projects" :key="p.id">
              <td>{{ p.id }}</td>
              <td>
                <span class="fw-semibold">{{ p.name }}</span>
                <span v-if="p.description" class="d-block small text-secondary">
                  {{ p.description }}
                </span>
              </td>
              <td><code>{{ p.code }}</code></td>
              <td>{{ p.owner_id ?? "—" }}</td>
              <td>
                <span v-if="p.enabled" class="badge bg-success-lt">启用</span>
                <span v-else class="badge bg-danger-lt">禁用</span>
              </td>
              <td v-for="f in cfDefs" :key="f.id" class="small">
                {{ cfCellText(p.id, f.id) }}
              </td>
              <td class="text-end text-nowrap">
                <div class="dropdown">
                  <button
                    type="button"
                    class="btn btn-sm btn-ghost-secondary dropdown-toggle d-inline-flex align-items-center gap-1"
                    data-bs-toggle="dropdown"
                    data-bs-config='{"popperConfig":{"strategy":"fixed"}}'
                    aria-expanded="false"
                  >
                    <IconDotsVertical :size="16" />
                    更多
                  </button>
                  <ul class="dropdown-menu dropdown-menu-end">
                    <li>
                      <button type="button" class="dropdown-item" @click="openEdit(p)" v-permission="['project:update']">
                        编辑
                      </button>
                    </li>
                    <li>
                      <button type="button" class="dropdown-item" @click="openMembers(p)">
                        成员管理
                      </button>
                    </li>
                    <li>
                      <hr class="dropdown-divider" />
                    </li>
                    <li>
                      <button
                        type="button"
                        class="dropdown-item text-danger"
                        @click="openDelete(p)"
                       v-permission="['project:delete','@super']">
                        删除
                      </button>
                    </li>
                  </ul>
                </div>
              </td>
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

    <!-- 新建/编辑项目 -->
    <CdModal :show="showEdit" :title="editing ? '编辑项目' : '新建项目'" size="md" @close="showEdit = false">
      <form novalidate @submit.prevent="saveProject">
        <div class="modal-body">
          <p v-if="formError" class="alert alert-danger py-2 small mb-3" role="alert">
            {{ formError }}
          </p>
          <div class="mb-2">
            <label class="form-label" :class="{ required: !editing }">项目编码</label>
            <input
              v-model="form.code"
              class="form-control"
              :disabled="!!editing"
              placeholder="如 video_prod"
              autocomplete="off"
            />
            <div v-if="editing" class="form-hint">编码创建后不可修改</div>
          </div>
          <div class="mb-2">
            <label class="form-label required">项目名称</label>
            <input v-model="form.name" class="form-control" placeholder="如 视频生产项目" />
          </div>
          <div class="mb-2">
            <label class="form-label">描述</label>
            <textarea v-model="form.description" class="form-control" rows="3" placeholder="可选"></textarea>
          </div>

          <!-- 扩展字段 -->
          <div v-if="cfDefs.length" class="mt-3 border-top pt-3">
            <h6 class="mb-3 text-secondary">扩展字段</h6>
            <div class="row g-3">
              <div
                v-for="f in cfDefs"
                :key="f.id"
                :class="f.field_type === 'textarea' ? 'col-12' : 'col-md-6'"
              >
                <label class="form-label" :class="{ required: !!f.required }">
                  {{ f.field_name }}
                </label>
                <template v-if="editing">
                  <input v-if="f.field_type === 'text'" v-model="cfEditValues[f.id]" class="form-control" />
                  <input v-else-if="f.field_type === 'number'" v-model="cfEditValues[f.id]" type="number" class="form-control" />
                  <input v-else-if="f.field_type === 'date'" v-model="cfEditValues[f.id]" type="date" class="form-control" />
                  <input v-else-if="f.field_type === 'datetime'" v-model="cfEditValues[f.id]" type="datetime-local" class="form-control" />
                  <textarea v-else-if="f.field_type === 'textarea'" v-model="cfEditValues[f.id]" class="form-control" rows="2"></textarea>
                  <select v-else-if="f.field_type === 'select'" v-model="cfEditValues[f.id]" class="form-select">
                    <option value="">请选择</option>
                    <option v-for="o in f.options || []" :key="o.id" :value="o.code">{{ o.name }}</option>
                  </select>
                  <select v-else-if="f.field_type === 'multi_select'" v-model="cfEditValues[f.id]" class="form-select" multiple size="4">
                    <option v-for="o in f.options || []" :key="o.id" :value="o.code">{{ o.name }}</option>
                  </select>
                </template>
                <template v-else>
                  <input v-if="f.field_type === 'text'" v-model="cfCreateValues[f.id]" class="form-control" />
                  <input v-else-if="f.field_type === 'number'" v-model="cfCreateValues[f.id]" type="number" class="form-control" />
                  <input v-else-if="f.field_type === 'date'" v-model="cfCreateValues[f.id]" type="date" class="form-control" />
                  <input v-else-if="f.field_type === 'datetime'" v-model="cfCreateValues[f.id]" type="datetime-local" class="form-control" />
                  <textarea v-else-if="f.field_type === 'textarea'" v-model="cfCreateValues[f.id]" class="form-control" rows="2"></textarea>
                  <select v-else-if="f.field_type === 'select'" v-model="cfCreateValues[f.id]" class="form-select">
                    <option value="">请选择</option>
                    <option v-for="o in f.options || []" :key="o.id" :value="o.code">{{ o.name }}</option>
                  </select>
                  <select v-else-if="f.field_type === 'multi_select'" v-model="cfCreateValues[f.id]" class="form-select" multiple size="4">
                    <option v-for="o in f.options || []" :key="o.id" :value="o.code">{{ o.name }}</option>
                  </select>
                </template>
                <div v-if="f.description" class="form-hint">{{ f.description }}</div>
              </div>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-sm" @click="showEdit = false">取消</button>
          <button type="submit" class="btn btn-sm btn-primary">保存</button>
        </div>
      </form>
    </CdModal>

    <!-- 删除确认 -->
    <CdModal :show="showDelete" title="删除项目" size="sm" @close="showDelete = false">
      <div class="modal-body">
        <p>确定要删除项目「{{ deleting?.name }}」吗？该操作将同时移除所有项目成员，不可恢复。</p>
      </div>
      <div class="modal-footer">
        <button class="btn btn-sm" @click="showDelete = false">取消</button>
        <button class="btn btn-sm btn-danger" @click="confirmDelete" v-permission="['project:delete','@super']">删除</button>
      </div>
    </CdModal>

    <!-- 成员管理 -->
    <CdModal :show="showMembers" :title="`项目成员 - ${memberProject?.name || ''}`" size="lg" @close="showMembers = false">
      <div class="modal-body">
        <div class="d-flex justify-content-end mb-2">
          <button class="btn btn-sm btn-primary" @click="showAddMember = true" v-permission="['project:update']">
            添加成员
          </button>
        </div>
        <div class="table-responsive">
          <table class="table table-vcenter card-table cd-admin-table">
            <thead>
              <tr>
                <th class="w-1">用户 ID</th>
                <th>用户名</th>
                <th>姓名</th>
                <th>角色</th>
                <th class="w-1">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="memberLoading">
                <td colspan="5" class="text-secondary text-center py-4">加载中…</td>
              </tr>
              <tr v-else-if="!members.length">
                <td colspan="5" class="text-secondary text-center py-4">暂无成员</td>
              </tr>
              <tr v-for="m in members" :key="m.id">
                <td>{{ m.user_id }}</td>
                <td>{{ m.username }}</td>
                <td>{{ m.real_name || "—" }}</td>
                <td>
                  <span class="badge bg-secondary-lt">{{ memberRoleLabel(m.role) }}</span>
                </td>
                <td class="text-end">
                  <div class="dropdown">
                    <button
                      type="button"
                      class="btn btn-sm btn-ghost-secondary dropdown-toggle d-inline-flex align-items-center gap-1"
                      data-bs-toggle="dropdown"
                    data-bs-config='{"popperConfig":{"strategy":"fixed"}}'
                      aria-expanded="false"
                    >
                      <IconDotsVertical :size="16" />
                      更多
                    </button>
                    <ul class="dropdown-menu dropdown-menu-end">
                      <li>
                        <button type="button" class="dropdown-item" @click="showAddMember = true" v-permission="['project:update']">
                          修改角色
                        </button>
                      </li>
                      <li>
                        <hr class="dropdown-divider" />
                      </li>
                      <li>
                        <button
                          type="button"
                          class="dropdown-item text-danger"
                          @click="removeMember(m)"
                         v-permission="['project:update']">
                          移除成员
                        </button>
                      </li>
                    </ul>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </CdModal>

    <!-- 添加成员 -->
    <CdModal :show="showAddMember" title="添加项目成员" size="sm" @close="showAddMember = false">
      <form novalidate @submit.prevent="addMember">
        <div class="modal-body">
          <div class="mb-2">
            <label class="form-label required">用户 ID</label>
            <input v-model="addMemberForm.user_id" type="number" class="form-control" placeholder="输入用户 ID" />
          </div>
          <div class="mb-2">
            <label class="form-label">角色</label>
            <select v-model="addMemberForm.role" class="form-select">
              <option v-for="o in memberRoleOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-sm" @click="showAddMember = false">取消</button>
          <button type="submit" class="btn btn-sm btn-primary">添加</button>
        </div>
      </form>
    </CdModal>
  </div>
</template>
