<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { IconDotsVertical } from "@tabler/icons-vue";
import { ApiError } from "@/api/client";
import * as rbacApi from "@/api/rbac";
import type { Role, PermissionModule } from "@/api/rbac";
import { dataScopeLabel, dataScopeOptions } from "@/api/rbac";
import { usePageSize } from "@/composables/usePageSize";
import { useToast } from "@/composables/useToast";
import CdModal from "@/components/CdModal.vue";
import CdPagination from "@/components/CdPagination.vue";

const toast = useToast();
const { pageSize, ensurePageSize } = usePageSize();

const roles = ref<Role[]>([]);
const loading = ref(false);
const keyword = ref("");
const page = ref(1);

const filteredRoles = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw) return roles.value;
  return roles.value.filter(
    (r) =>
      r.name.toLowerCase().includes(kw) ||
      r.code.toLowerCase().includes(kw) ||
      (r.description || "").toLowerCase().includes(kw),
  );
});

const total = computed(() => filteredRoles.value.length);
const pagedRoles = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return filteredRoles.value.slice(start, start + pageSize.value);
});

watch(page, () => {});

function search() {
  page.value = 1;
}

const permissionModules = ref<PermissionModule[]>([]);

// ---------- 新建/编辑角色 ----------
const showEdit = ref(false);
const editing = ref<Role | null>(null);
const form = ref({
  code: "",
  name: "",
  description: "",
  data_scope: "self",
});
const formError = ref("");

function openCreate() {
  editing.value = null;
  form.value = { code: "", name: "", description: "", data_scope: "self" };
  formError.value = "";
  showEdit.value = true;
}

function openEdit(r: Role) {
  editing.value = r;
  form.value = {
    code: r.code,
    name: r.name,
    description: r.description || "",
    data_scope: r.data_scope,
  };
  formError.value = "";
  showEdit.value = true;
}

async function saveRole() {
  formError.value = "";
  if (!form.value.name.trim()) {
    formError.value = "名称不能为空";
    return;
  }
  if (!editing.value && !form.value.code.trim()) {
    formError.value = "编码不能为空";
    return;
  }
  try {
    if (editing.value) {
      await rbacApi.updateRole(editing.value.id, {
        name: form.value.name,
        description: form.value.description,
        data_scope: form.value.data_scope,
      });
      toast.show("已更新");
    } else {
      await rbacApi.createRole({
        code: form.value.code,
        name: form.value.name,
        description: form.value.description,
        data_scope: form.value.data_scope,
      });
      toast.show("已创建");
    }
    showEdit.value = false;
    await loadRoles();
  } catch (e) {
    formError.value = e instanceof ApiError ? e.message : "保存失败";
  }
}

// ---------- 删除角色 ----------
const showDelete = ref(false);
const deleting = ref<Role | null>(null);

function openDelete(r: Role) {
  deleting.value = r;
  showDelete.value = true;
}

async function confirmDelete() {
  if (!deleting.value) return;
  try {
    await rbacApi.deleteRole(deleting.value.id);
    toast.show("已删除");
    showDelete.value = false;
    await loadRoles();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "删除失败");
  }
}

// ---------- 分配权限 ----------
const showPerm = ref(false);
const permRole = ref<Role | null>(null);
const checkedPerms = ref<Set<string>>(new Set());
const permSaving = ref(false);

async function openPermissions(r: Role) {
  permRole.value = r;
  checkedPerms.value = new Set();
  try {
    const res = await rbacApi.getRolePermissions(r.id);
    checkedPerms.value = new Set(res.permissions);
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载权限失败");
  }
  showPerm.value = true;
}

function togglePerm(code: string) {
  const next = new Set(checkedPerms.value);
  if (next.has(code)) next.delete(code);
  else next.add(code);
  checkedPerms.value = next;
}

function toggleModule(module: PermissionModule) {
  const moduleCodes = module.items.map((i) => i.code);
  const allChecked = moduleCodes.every((c) => checkedPerms.value.has(c));
  const next = new Set(checkedPerms.value);
  if (allChecked) {
    moduleCodes.forEach((c) => next.delete(c));
  } else {
    moduleCodes.forEach((c) => next.add(c));
  }
  checkedPerms.value = next;
}

function moduleCheckedCount(m: PermissionModule) {
  return m.items.filter((i) => checkedPerms.value.has(i.code)).length;
}

async function savePermissions() {
  if (!permRole.value) return;
  permSaving.value = true;
  try {
    await rbacApi.assignRolePermissions(permRole.value.id, Array.from(checkedPerms.value));
    toast.show("权限已更新");
    showPerm.value = false;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  } finally {
    permSaving.value = false;
  }
}

// ---------- 加载 ----------
async function loadRoles() {
  loading.value = true;
  try {
    const res = await rbacApi.listRoles();
    roles.value = res.items;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function loadPermissions() {
  try {
    const res = await rbacApi.listPermissions();
    permissionModules.value = res.modules;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载权限失败");
  }
}

onMounted(async () => {
  await ensurePageSize();
  loadRoles();
  loadPermissions();
});
</script>

<template>
  <div>
    <div class="d-flex gap-2 mb-3 flex-wrap align-items-center">
      <input
        v-model="keyword"
        class="form-control form-control-sm"
        placeholder="搜索角色名称/编码"
        style="max-width: 14rem"
        @keyup.enter="search"
      />

      <button type="button" class="btn btn-sm btn-primary" @click="openCreate" v-permission="['role:create','@super']">
        新建角色
      </button>
      <button type="button" class="btn btn-sm" @click="loadRoles">刷新</button>
    </div>

    <div class="card">
      <div class="table-responsive">
        <table class="table table-vcenter card-table cd-admin-table">
          <thead>
            <tr>
              <th class="w-1">ID</th>
              <th>角色名称</th>
              <th>编码</th>
              <th>数据范围</th>
              <th>类型</th>
              <th>状态</th>
              <th class="w-1"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td colspan="7" class="text-secondary text-center py-4">加载中…</td>
            </tr>
            <tr v-else-if="!roles.length">
              <td colspan="7" class="text-secondary text-center py-4">暂无角色</td>
            </tr>
            <tr v-else-if="!pagedRoles.length">
              <td colspan="7" class="text-secondary text-center py-4">
                未找到匹配的角色
              </td>
            </tr>
            <tr v-for="r in pagedRoles" :key="r.id">
              <td>{{ r.id }}</td>
              <td>
                <span class="fw-semibold">{{ r.name }}</span>
                <span v-if="r.description" class="d-block small text-secondary">
                  {{ r.description }}
                </span>
              </td>
              <td><code>{{ r.code }}</code></td>
              <td>{{ dataScopeLabel(r.data_scope) }}</td>
              <td>
                <span v-if="r.is_builtin" class="badge bg-azure-lt">内置</span>
                <span v-else class="badge bg-secondary-lt">自定义</span>
              </td>
              <td>
                <span v-if="r.enabled" class="badge bg-success-lt">启用</span>
                <span v-else class="badge bg-danger-lt">禁用</span>
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
                      <button type="button" class="dropdown-item" @click="openEdit(r)" v-permission="['role:update','@super']">
                        编辑
                      </button>
                    </li>
                    <li>
                      <button
                        type="button"
                        class="dropdown-item"
                        @click="openPermissions(r)"
                       v-permission="['role:assign','@super']">
                        分配权限
                      </button>
                    </li>
                    <li>
                      <hr class="dropdown-divider" />
                    </li>
                    <li>
                      <button
                        type="button"
                        class="dropdown-item text-danger"
                        :disabled="r.is_builtin"
                        @click="openDelete(r)"
                       v-permission="['role:delete','@super']">
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

    <!-- 新建/编辑弹窗 -->
    <CdModal :show="showEdit" :title="editing ? '编辑角色' : '新建角色'" size="md" @close="showEdit = false">
      <form novalidate @submit.prevent="saveRole">
        <div class="modal-body">
          <p v-if="formError" class="alert alert-danger py-2 small mb-3" role="alert">
            {{ formError }}
          </p>
          <div class="mb-2">
            <label class="form-label" :class="{ required: !editing }">角色编码</label>
            <input
              v-model="form.code"
              class="form-control"
              :disabled="!!editing"
              placeholder="如 content_manager"
              autocomplete="off"
            />
            <div v-if="editing" class="form-hint">编码创建后不可修改</div>
          </div>
          <div class="mb-2">
            <label class="form-label required">角色名称</label>
            <input v-model="form.name" class="form-control" placeholder="如 内容管理员" />
          </div>
          <div class="mb-2">
            <label class="form-label">数据范围</label>
            <select v-model="form.data_scope" class="form-select">
              <option v-for="o in dataScopeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
          <div class="mb-2">
            <label class="form-label">描述</label>
            <textarea v-model="form.description" class="form-control" rows="3" placeholder="可选"></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-sm" @click="showEdit = false">取消</button>
          <button type="submit" class="btn btn-sm btn-primary">保存</button>
        </div>
      </form>
    </CdModal>

    <!-- 删除确认 -->
    <CdModal :show="showDelete" title="删除角色" size="sm" @close="showDelete = false">
      <div class="modal-body">
        <p>确定要删除角色「{{ deleting?.name }}」吗？此操作不可恢复。</p>
      </div>
      <div class="modal-footer">
        <button class="btn btn-sm" @click="showDelete = false">取消</button>
        <button class="btn btn-sm btn-danger" @click="confirmDelete" v-permission="['role:delete','@super']">删除</button>
      </div>
    </CdModal>

    <!-- 分配权限弹窗 -->
    <CdModal :show="showPerm" :title="`分配权限 - ${permRole?.name || ''}`" size="lg" @close="showPerm = false">
      <div class="modal-body">
        <div class="cd-perm-tree">
          <div v-for="m in permissionModules" :key="m.module" class="cd-perm-module">
            <div class="cd-perm-module-head">
              <label class="cd-perm-check">
                <input
                  type="checkbox"
                  :checked="moduleCheckedCount(m) === m.items.length"
                  :indeterminate.prop="moduleCheckedCount(m) > 0 && moduleCheckedCount(m) < m.items.length"
                  @change="toggleModule(m)"
                />
                <span class="cd-perm-module-name">{{ m.module }}</span>
                <span class="cd-perm-count">{{ moduleCheckedCount(m) }}/{{ m.items.length }}</span>
              </label>
            </div>
            <div class="cd-perm-items">
              <label v-for="p in m.items" :key="p.code" class="cd-perm-item">
                <input
                  type="checkbox"
                  :checked="checkedPerms.has(p.code)"
                  @change="togglePerm(p.code)"
                />
                <span>{{ p.name }}</span>
                <code class="cd-perm-code">{{ p.code }}</code>
              </label>
            </div>
          </div>
          <p v-if="permissionModules.length === 0" class="text-secondary text-center py-4">暂无权限点</p>
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn btn-sm" @click="showPerm = false">取消</button>
        <button class="btn btn-sm btn-primary" :disabled="permSaving" @click="savePermissions" v-permission="['role:assign','@super']">
          {{ permSaving ? "保存中…" : "保存权限" }}
        </button>
      </div>
    </CdModal>
  </div>
</template>

<style scoped>
.cd-perm-tree {
  max-height: 60vh;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.cd-perm-module {
  border: 1px solid var(--tblr-border-color);
  border-radius: 8px;
  overflow: hidden;
}
.cd-perm-module-head {
  padding: 10px 12px;
  background: var(--tblr-bg-surface-secondary);
}
.cd-perm-check {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  font-weight: 600;
}
.cd-perm-module-name {
  text-transform: capitalize;
}
.cd-perm-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--tblr-secondary);
  font-weight: 400;
}
.cd-perm-items {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 6px;
  padding: 10px 12px;
}
.cd-perm-item {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  font-size: 13px;
  padding: 4px 6px;
  border-radius: 4px;
}
.cd-perm-item:hover {
  background: var(--tblr-bg-surface-secondary);
}
.cd-perm-code {
  margin-left: auto;
  font-size: 11px;
  color: var(--tblr-secondary);
}
</style>
