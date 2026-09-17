<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { ApiError } from "@/api/client";
import * as usersApi from "@/api/users";
import type { UserRow } from "@/api/users";
import * as deptApi from "@/api/departments";
import type { DepartmentRow } from "@/api/departments";
import * as authApi from "@/api/auth";
import * as cfApi from "@/api/customFields";
import type { CustomFieldWithValue } from "@/api/customFields";
import { isValidEmail, isValidUsername } from "@/utils/validate";
import { useAuthStore } from "@/stores/auth";
import { usePageSize } from "@/composables/usePageSize";
import { useToast } from "@/composables/useToast";
import CdModal from "@/components/CdModal.vue";
import CdPagination from "@/components/CdPagination.vue";
import CdAvatar from "@/components/CdAvatar.vue";
import { IconDotsVertical } from "@tabler/icons-vue";

const auth = useAuthStore();
const toast = useToast();
const { pageSize, ensurePageSize } = usePageSize();
const items = ref<UserRow[]>([]);
const departments = ref<DepartmentRow[]>([]);
const total = ref(0);
const page = ref(1);
const keyword = ref("");
const loading = ref(false);
const passwordRules = ref<string[]>([]);

const showCreate = ref(false);
const showEdit = ref(false);
const showChangePassword = ref(false);
const editing = ref<UserRow | null>(null);
const changePasswordUser = ref<UserRow | null>(null);
const form = ref({
  username: "",
  email: "",
  password: "",
  role: "user",
  real_name: "",
  department_id: "" as string | number,
});
const editForm = ref({
  email: "",
  role: "user",
  real_name: "",
  department_id: "" as string | number,
});
const changePasswordForm = ref({ new_password: "", new_password2: "" });

const createTouched = ref({ username: false, email: false, password: false });
const editTouched = ref({ email: false });
const changePasswordTouched = ref({ password: false });
const creating = ref(false);
const saving = ref(false);
const changingPassword = ref(false);

// 自定义字段（user 模块）。createModal 与 editModal 共享同一字段定义列表，
// createValues/editValues 分别保存表单中绑定的当前值。
const cfDefs = ref<CustomFieldWithValue[]>([]);
const cfCreateValues = ref<Record<number, string | string[]>>({});
const cfEditValues = ref<Record<number, string | string[]>>({});
const cfLoading = ref(false);
// 列表批量值：cfListValues[userId][fieldId] = raw string
const cfListValues = ref<Record<number, Record<number, string>>>({});

const CUSTOM_FIELD_MODULE = "user";

const passwordRulesHint = computed(() => {
  if (!passwordRules.value.length) return "至少 5 位";
  return passwordRules.value.join("、");
});

const createUsernameError = computed(() => {
  if (!createTouched.value.username) return "";
  const v = form.value.username.trim();
  if (!v) return "请填写用户名";
  if (v.length < 3) return "用户名至少 3 个字符";
  if (v.length > 64) return "用户名过长";
  if (!isValidUsername(v)) return "用户名仅允许字母、数字、点、下划线、短横线";
  return "";
});

const createEmailError = computed(() => {
  if (!createTouched.value.email) return "";
  const v = form.value.email.trim();
  if (!v) return "请填写邮箱";
  if (!isValidEmail(v)) return "邮箱格式不正确，例如 name@example.com";
  return "";
});

const createPasswordError = computed(() => {
  if (!createTouched.value.password) return "";
  const v = form.value.password;
  if (!v) return "请填写密码";
  if (v.length < 5) return "密码至少 5 位";
  return "";
});

const editEmailError = computed(() => {
  if (!editTouched.value.email || editing.value?.is_system) return "";
  const v = editForm.value.email.trim();
  if (!v) return "请填写邮箱";
  if (!isValidEmail(v)) return "邮箱格式不正确，例如 name@example.com";
  return "";
});

const changePasswordError = computed(() => {
  const pwd = changePasswordForm.value.new_password;
  const pwd2 = changePasswordForm.value.new_password2;
  if (!changePasswordTouched.value.password && !pwd && !pwd2) return "";
  if (!pwd) return "请填写新密码";
  if (pwd.length < 5) return "密码至少 5 位";
  if (pwd !== pwd2) return "两次输入的密码不一致";
  return "";
});

const createFormErrorSummary = computed(() => {
  const parts = [
    createUsernameError.value,
    createEmailError.value,
    createPasswordError.value,
  ].filter(Boolean);
  return parts.length ? parts.join("；") : "";
});

function touchCreate(field: "username" | "email" | "password") {
  createTouched.value = { ...createTouched.value, [field]: true };
}

function resetCreateForm() {
  form.value = {
    username: "",
    email: "",
    password: "",
    role: "user",
    real_name: "",
    department_id: "",
  };
  createTouched.value = { username: false, email: false, password: false };
  cfCreateValues.value = {};
}

async function openCreate() {
  resetCreateForm();
  showCreate.value = true;
  if (!cfDefs.value.length) {
    await loadCustomFieldDefs();
  }
  // 给每个字段填默认值（multi_select 用空数组，其余空字符串）
  const next: Record<number, string | string[]> = {};
  for (const d of cfDefs.value) {
    next[d.id] = d.field_type === "multi_select" ? [] : "";
  }
  cfCreateValues.value = next;
}

async function loadDepts() {
  try {
    departments.value = await deptApi.listAllDepartments();
  } catch {
    departments.value = [];
  }
}

async function loadPasswordRules() {
  try {
    const cfg = await authApi.fetchAuthConfig();
    passwordRules.value = cfg.password_rules || [];
  } catch {
    passwordRules.value = [];
  }
}

// 自定义字段：加载该模块的启用字段定义。
// 仅刷新定义列表（createValues 仍由 openCreate 清空、openEdit 时按值回填）。
async function loadCustomFieldDefs() {
  try {
    const res = await cfApi.getCustomFieldDefinitions(CUSTOM_FIELD_MODULE);
    cfDefs.value = res.fields || [];
  } catch {
    cfDefs.value = [];
  }
}

// 自定义字段：加载某实体的字段定义 + 当前值，并回填到 editValues。
async function loadCustomFieldValues(userId: number) {
  if (!cfDefs.value.length) {
    await loadCustomFieldDefs();
  }
  cfEditValues.value = {};
  if (!cfDefs.value.length || !userId) return;
  cfLoading.value = true;
  try {
    const res = await cfApi.getCustomFieldValues(CUSTOM_FIELD_MODULE, userId);
    // 用最新定义为准，按返回字段补值；缺失字段补默认值
    const byId = new Map<number, CustomFieldWithValue>();
    for (const f of res.fields || []) byId.set(f.id, f);
    cfDefs.value = cfDefs.value.map((d) => byId.get(d.id) || d);
    const next: Record<number, string | string[]> = {};
    for (const d of cfDefs.value) {
      next[d.id] = cfApi.decodeFieldValue(byId.get(d.id)?.value || "", d.field_type);
    }
    cfEditValues.value = next;
  } catch {
    // 读取失败时给字段填默认值，不阻塞编辑
    const next: Record<number, string | string[]> = {};
    for (const d of cfDefs.value) {
      next[d.id] = d.field_type === "multi_select" ? [] : "";
    }
    cfEditValues.value = next;
  } finally {
    cfLoading.value = false;
  }
}

// 把 createValues/editValues 序列化为提交给后端的 values 列表。
function serializeCfValues(values: Record<number, string | string[]>): { field_id: number; value: string }[] {
  return cfDefs.value.map((d) => ({
    field_id: d.id,
    value: cfApi.encodeFieldValue(values[d.id] ?? (d.field_type === "multi_select" ? [] : ""), d.field_type),
  }));
}

// 必填校验：required 字段不能为空字符串/空数组。
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

// 创建场景下保存自定义字段（用户已创建成功后调用，失败仅提示不阻塞主流程）。
async function saveCfAfterCreate(userId: number) {
  if (!cfDefs.value.length) return;
  const payload = serializeCfValues(cfCreateValues.value);
  if (!payload.length) return;
  try {
    await cfApi.saveCustomFieldValues(CUSTOM_FIELD_MODULE, userId, payload);
  } catch (e) {
    toast.show(e instanceof ApiError ? `自定义字段保存失败：${e.message}` : "自定义字段保存失败");
  }
}

// 编辑场景下保存自定义字段。
async function saveCfAfterEdit(userId: number) {
  if (!cfDefs.value.length) return;
  const payload = serializeCfValues(cfEditValues.value);
  if (!payload.length) return;
  try {
    await cfApi.saveCustomFieldValues(CUSTOM_FIELD_MODULE, userId, payload);
  } catch (e) {
    toast.show(e instanceof ApiError ? `自定义字段保存失败：${e.message}` : "自定义字段保存失败");
  }
}

// 列表加载后批量拉取自定义字段值，合并到 cfListValues 供列表列渲染。
async function loadCustomFieldBatchValues() {
  if (!cfDefs.value.length) {
    await loadCustomFieldDefs();
  }
  if (!cfDefs.value.length || !items.value.length) {
    cfListValues.value = {};
    return;
  }
  const ids = items.value.map((u) => u.id);
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

// 列表单元格格式化：把某个用户某个字段的 raw 值格式化为可读文本。
function cfCellText(userId: number, fieldId: number): string {
  const def = cfDefs.value.find((d) => d.id === fieldId);
  if (!def) return "—";
  const raw = cfListValues.value[userId]?.[fieldId] || "";
  return cfApi.formatFieldValue(raw, def.field_type, def.options);
}

// 详情弹窗状态
const showDetail = ref(false);
const detailUser = ref<UserRow | null>(null);
const detailCfValues = ref<CustomFieldWithValue[]>([]);
const detailCfLoading = ref(false);

async function openDetail(u: UserRow) {
  detailUser.value = u;
  showDetail.value = true;
  detailCfLoading.value = true;
  detailCfValues.value = [];
  try {
    const res = await cfApi.getCustomFieldValues(CUSTOM_FIELD_MODULE, u.id);
    detailCfValues.value = res.fields || [];
  } catch {
    detailCfValues.value = [];
  } finally {
    detailCfLoading.value = false;
  }
}

function detailCfText(field: CustomFieldWithValue): string {
  return cfApi.formatFieldValue(field.value || "", field.field_type, field.options);
}

async function load() {
  loading.value = true;
  try {
    const res = await usersApi.listUsers({
      page: page.value,
      page_size: pageSize.value,
      keyword: keyword.value || undefined,
    });
    items.value = res.items;
    total.value = res.total;
    // 拉取本页用户的自定义字段值（不阻塞主列表渲染）
    loadCustomFieldBatchValues();
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

watch(page, () => load());

function validateCreate(): boolean {
  createTouched.value = { username: true, email: true, password: true };
  return (
    !createUsernameError.value &&
    !createEmailError.value &&
    !createPasswordError.value
  );
}

async function create() {
  if (!validateCreate()) {
    toast.show(
      createUsernameError.value ||
        createEmailError.value ||
        createPasswordError.value ||
        "请检查表单",
    );
    return;
  }
  if (!validateCfRequired(cfCreateValues.value)) {
    return;
  }
  creating.value = true;
  try {
    const created = await usersApi.createUser({
      username: form.value.username.trim(),
      email: form.value.email.trim(),
      password: form.value.password,
      role: form.value.role,
      real_name: form.value.real_name || undefined,
      department_id: form.value.department_id
        ? Number(form.value.department_id)
        : null,
    });
    // 自定义字段保存失败不回滚用户，仅提示
    await saveCfAfterCreate(created.id);
    showCreate.value = false;
    resetCreateForm();
    toast.show("已创建");
    load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "创建失败");
  } finally {
    creating.value = false;
  }
}

async function openEdit(u: UserRow) {
  editing.value = u;
  editForm.value = {
    email: u.email || "",
    role: u.role,
    real_name: u.real_name || "",
    department_id: u.department_id ?? "",
  };
  editTouched.value = { email: false };
  showEdit.value = true;
  await loadCustomFieldValues(u.id);
}

function openChangePassword(u: UserRow) {
  if (u.is_system) return toast.show("系统用户不允许修改密码");
  changePasswordUser.value = u;
  changePasswordForm.value = { new_password: "", new_password2: "" };
  changePasswordTouched.value = { password: false };
  showChangePassword.value = true;
}

async function saveChangePassword() {
  if (!changePasswordUser.value) return;
  changePasswordTouched.value = { password: true };
  if (changePasswordError.value) {
    toast.show(changePasswordError.value);
    return;
  }
  changingPassword.value = true;
  try {
    await usersApi.updateUser(changePasswordUser.value.id, {
      new_password: changePasswordForm.value.new_password,
    });
    showChangePassword.value = false;
    changePasswordUser.value = null;
    toast.show("密码已更新");
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "修改密码失败");
  } finally {
    changingPassword.value = false;
  }
}

async function toggleDisabled(u: UserRow) {
  if (u.is_system) return toast.show("系统用户不允许禁用");
  try {
    await usersApi.setUserDisabled(u.id, !u.disabled);
    load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "操作失败");
  }
}

async function remove(u: UserRow) {
  if (!auth.isSuperAdmin) return toast.show("需要超级管理员权限");
  if (u.is_system) return toast.show("系统用户不允许删除");
  if (u.id === auth.user?.user_id) return toast.show("不能删除自己");
  if (!confirm(`删除用户 ${u.username}？`)) return;
  try {
    await usersApi.deleteUser(u.id);
    toast.show("已删除");
    load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "删除失败");
  }
}

async function saveEdit() {
  if (!editing.value) return;
  if (!editing.value.is_system) {
    editTouched.value = { email: true };
    if (editEmailError.value) {
      toast.show(editEmailError.value);
      return;
    }
  }
  if (!validateCfRequired(cfEditValues.value)) {
    return;
  }
  saving.value = true;
  try {
    const payload: Parameters<typeof usersApi.updateUser>[1] = {
      email: editForm.value.email.trim(),
      real_name: editForm.value.real_name || undefined,
      department_id: editForm.value.department_id
        ? Number(editForm.value.department_id)
        : 0,
    };
    if (!editing.value.is_system) {
      payload.role = editForm.value.role;
    } else {
      delete payload.email;
    }
    await usersApi.updateUser(editing.value.id, payload);
    await saveCfAfterEdit(editing.value.id);
    showEdit.value = false;
    toast.show("已保存");
    load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

onMounted(async () => {
  await ensurePageSize();
  await loadDepts();
  await loadPasswordRules();
  loadCustomFieldDefs();
  load();
});
</script>

<template>
  <div>
    <div class="d-flex gap-2 mb-3 flex-wrap align-items-center">
      <input
        v-model="keyword"
        class="form-control form-control-sm"
        placeholder="搜索用户"
        style="max-width: 14rem"
        @keyup.enter="search"
      />

      <button type="button" class="btn btn-sm btn-primary" @click="openCreate" v-permission="['user:create']">
        添加用户
      </button>
      <button type="button" class="btn btn-sm" @click="load">刷新</button>
    </div>

    <div class="card">
      <div class="table-responsive">
        <table class="table table-vcenter card-table cd-admin-table">
          <thead>
            <tr>
              <th></th>
              <th>用户名</th>
              <th>邮箱</th>
              <th>来源</th>
              <th>角色</th>
              <th>部门</th>
              <th>状态</th>
              <th>显示名</th>
              <th v-for="f in cfDefs" :key="f.id">{{ f.field_name }}</th>
              <th class="w-1"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="u in items" :key="u.id">
              <td class="w-1">
                <CdAvatar
                  :src="u.avatar_url"
                  :name="u.real_name || u.username"
                  kind="user"
                  size="sm"
                />
              </td>
              <td>{{ u.username }}</td>
              <td>{{ u.email || "—" }}</td>
              <td>
                <span v-if="u.auth_source === 'ldap'" class="badge bg-azure-lt"
                  >LDAP</span
                >
                <span v-else class="badge bg-secondary-lt">本地</span>
              </td>
              <td>
                <span class="badge bg-secondary-lt">{{ u.role }}</span>
              </td>
              <td>{{ u.department_name || "—" }}</td>
              <td>{{ u.disabled ? "禁用" : "正常" }}</td>
              <td>{{ u.real_name || "—" }}</td>
              <td v-for="f in cfDefs" :key="f.id" class="small">
                {{ cfCellText(u.id, f.id) }}
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
                      <button
                        type="button"
                        class="dropdown-item"
                        @click="openDetail(u)"
                      >
                        详情
                      </button>
                    </li>
                    <li>
                      <button
                        type="button"
                        class="dropdown-item"
                        :disabled="
                          u.id === auth.user?.user_id ||
                          (u.role === 'superadmin' && !auth.isSuperAdmin)
                        "
                        @click="openEdit(u)"
                       v-permission="['user:update']">
                        编辑
                      </button>
                    </li>

                    <template v-if="!u.is_system">
                      <li><hr class="dropdown-divider" /></li>
                      <li>
                        <button
                          type="button"
                          class="dropdown-item"
                          @click="openChangePassword(u)"
                         v-permission="['user:update']">
                          修改密码
                        </button>
                      </li>

                      <li>
                        <button
                          type="button"
                          class="dropdown-item"
                          @click="toggleDisabled(u)"
                         v-permission="['user:disable']">
                          {{ u.disabled ? "启用账号" : "禁用账号" }}
                        </button>
                      </li>
                      <li v-if="auth.isSuperAdmin">
                        <hr class="dropdown-divider" />
                      </li>
                      <li v-if="auth.isSuperAdmin">
                        <button
                          type="button"
                          class="dropdown-item text-danger"
                          @click="remove(u)"
                         v-permission="['user:delete','@super']">
                          删除
                        </button>
                      </li>
                    </template>
                  </ul>
                </div>
              </td>
            </tr>
            <tr v-if="!items.length && !loading">
              <td :colspan="9 + cfDefs.length" class="text-secondary text-center py-4">
                暂无用户
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

    <CdModal
      :show="showCreate"
      title="添加用户"
      size="lg"
      @close="showCreate = false"
    >
      <form novalidate @submit.prevent="create">
        <div class="modal-body">
          <div
            v-if="createFormErrorSummary"
            class="alert alert-danger py-2 small mb-3"
            role="alert"
          >
            {{ createFormErrorSummary }}
          </div>
          <div class="row g-3">
            <div class="col-md-6">
              <label class="form-label required">用户名</label>
              <input
                v-model="form.username"
                class="form-control"
                :class="{ 'is-invalid': !!createUsernameError }"
                maxlength="64"
                autocomplete="off"
                placeholder="例如 alice"
                @input="touchCreate('username')"
                @blur="touchCreate('username')"
              />
              <div v-if="createUsernameError" class="invalid-feedback d-block">
                {{ createUsernameError }}
              </div>
              <div v-else class="form-hint">
                必填，3–64 位，仅字母、数字、点、下划线、短横线
              </div>
            </div>
            <div class="col-md-6">
              <label class="form-label required">邮箱</label>
              <input
                v-model="form.email"
                type="text"
                inputmode="email"
                class="form-control"
                :class="{ 'is-invalid': !!createEmailError }"
                maxlength="128"
                autocomplete="off"
                placeholder="name@example.com"
                @input="touchCreate('email')"
                @blur="touchCreate('email')"
              />
              <div v-if="createEmailError" class="invalid-feedback d-block">
                {{ createEmailError }}
              </div>
              <div v-else class="form-hint">
                必填，需为有效邮箱，例如 name@example.com
              </div>
            </div>
            <div class="col-md-6">
              <label class="form-label required">密码</label>
              <input
                v-model="form.password"
                type="password"
                class="form-control"
                :class="{ 'is-invalid': !!createPasswordError }"
                autocomplete="new-password"
                @input="touchCreate('password')"
                @blur="touchCreate('password')"
              />
              <div v-if="createPasswordError" class="invalid-feedback d-block">
                {{ createPasswordError }}
              </div>
              <div v-else class="form-hint">
                密码要求：{{ passwordRulesHint }}
              </div>
            </div>
            <div class="col-md-6">
              <label class="form-label">角色</label>
              <select v-model="form.role" class="form-select">
                <option value="user">user</option>
                <option value="admin">admin</option>
              </select>
            </div>
            <div class="col-md-6">
              <label class="form-label">部门</label>
              <select v-model="form.department_id" class="form-select">
                <option value="">未分配</option>
                <option v-for="d in departments" :key="d.id" :value="d.id">
                  {{ d.name }}
                </option>
              </select>
            </div>
            <div class="col-md-6">
              <label class="form-label">显示名</label>
              <input v-model="form.real_name" class="form-control" />
            </div>
          </div>

          <!-- 自定义字段 -->
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

                <!-- text -->
                <input
                  v-if="f.field_type === 'text'"
                  v-model="cfCreateValues[f.id]"
                  class="form-control"
                />

                <!-- number -->
                <input
                  v-else-if="f.field_type === 'number'"
                  v-model="cfCreateValues[f.id]"
                  type="number"
                  class="form-control"
                />

                <!-- date -->
                <input
                  v-else-if="f.field_type === 'date'"
                  v-model="cfCreateValues[f.id]"
                  type="date"
                  class="form-control"
                />

                <!-- datetime -->
                <input
                  v-else-if="f.field_type === 'datetime'"
                  v-model="cfCreateValues[f.id]"
                  type="datetime-local"
                  class="form-control"
                />

                <!-- textarea -->
                <textarea
                  v-else-if="f.field_type === 'textarea'"
                  v-model="cfCreateValues[f.id]"
                  class="form-control"
                  rows="2"
                ></textarea>

                <!-- select -->
                <select
                  v-else-if="f.field_type === 'select'"
                  v-model="cfCreateValues[f.id]"
                  class="form-select"
                >
                  <option value="">请选择</option>
                  <option v-for="o in f.options || []" :key="o.id" :value="o.code">
                    {{ o.name }}
                  </option>
                </select>

                <!-- multi_select -->
                <select
                  v-else-if="f.field_type === 'multi_select'"
                  v-model="cfCreateValues[f.id]"
                  class="form-select"
                  multiple
                  size="4"
                >
                  <option v-for="o in f.options || []" :key="o.id" :value="o.code">
                    {{ o.name }}
                  </option>
                </select>

                <div v-if="f.description" class="form-hint">{{ f.description }}</div>
              </div>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-sm" @click="showCreate = false">
            取消
          </button>
          <button
            type="submit"
            class="btn btn-sm btn-primary"
            :disabled="creating"
          >
            创建
          </button>
        </div>
      </form>
    </CdModal>

    <CdModal :show="showEdit" title="编辑用户" @close="showEdit = false">
      <form novalidate @submit.prevent="saveEdit">
        <div class="modal-body">
          <p class="text-secondary small">用户：{{ editing?.username }}</p>
          <div
            v-if="editEmailError"
            class="alert alert-danger py-2 small mb-3"
            role="alert"
          >
            {{ editEmailError }}
          </div>

          <div class="mb-2">
            <label class="form-label required">邮箱</label>
            <input
              v-model="editForm.email"
              type="text"
              inputmode="email"
              class="form-control"
              :class="{ 'is-invalid': !!editEmailError }"
              maxlength="128"
              placeholder="name@example.com"
              :disabled="!!editing?.is_system"
              @input="editTouched.email = true"
              @blur="editTouched.email = true"
            />
            <div v-if="editEmailError" class="invalid-feedback d-block">
              {{ editEmailError }}
            </div>
            <div
              v-else-if="editing?.is_system"
              class="form-hint text-secondary"
            >
              系统用户邮箱不可修改
            </div>
            <div v-else class="form-hint">
              需为有效邮箱，例如 name@example.com
            </div>
          </div>
          <div class="mb-2">
            <label class="form-label">角色</label>
            <select
              v-model="editForm.role"
              class="form-select"
              :disabled="!!editing?.is_system"
            >
              <option value="user">user</option>
              <option value="admin">admin</option>
              <option v-if="editing?.role === 'superadmin'" value="superadmin">
                superadmin
              </option>
            </select>
            <div v-if="editing?.is_system" class="form-hint text-secondary">
              系统用户不允许修改角色
            </div>
          </div>
          <div class="mb-2">
            <label class="form-label">部门</label
            ><select v-model="editForm.department_id" class="form-select">
              <option value="">未分配</option>
              <option v-for="d in departments" :key="d.id" :value="d.id">
                {{ d.name }}
              </option>
            </select>
          </div>
          <div class="mb-2">
            <label class="form-label">显示名</label
            ><input v-model="editForm.real_name" class="form-control" />
          </div>

          <!-- 自定义字段 -->
          <div v-if="cfDefs.length" class="mt-3 border-top pt-3">
            <h6 class="mb-3 text-secondary">
              扩展字段
              <span v-if="cfLoading" class="text-secondary small ms-1">加载中…</span>
            </h6>
            <div class="row g-3">
              <div
                v-for="f in cfDefs"
                :key="f.id"
                :class="f.field_type === 'textarea' ? 'col-12' : 'col-md-6'"
              >
                <label class="form-label" :class="{ required: !!f.required }">
                  {{ f.field_name }}
                </label>

                <input
                  v-if="f.field_type === 'text'"
                  v-model="cfEditValues[f.id]"
                  class="form-control"
                />
                <input
                  v-else-if="f.field_type === 'number'"
                  v-model="cfEditValues[f.id]"
                  type="number"
                  class="form-control"
                />
                <input
                  v-else-if="f.field_type === 'date'"
                  v-model="cfEditValues[f.id]"
                  type="date"
                  class="form-control"
                />
                <input
                  v-else-if="f.field_type === 'datetime'"
                  v-model="cfEditValues[f.id]"
                  type="datetime-local"
                  class="form-control"
                />
                <textarea
                  v-else-if="f.field_type === 'textarea'"
                  v-model="cfEditValues[f.id]"
                  class="form-control"
                  rows="2"
                ></textarea>
                <select
                  v-else-if="f.field_type === 'select'"
                  v-model="cfEditValues[f.id]"
                  class="form-select"
                >
                  <option value="">请选择</option>
                  <option v-for="o in f.options || []" :key="o.id" :value="o.code">
                    {{ o.name }}
                  </option>
                </select>
                <select
                  v-else-if="f.field_type === 'multi_select'"
                  v-model="cfEditValues[f.id]"
                  class="form-select"
                  multiple
                  size="4"
                >
                  <option v-for="o in f.options || []" :key="o.id" :value="o.code">
                    {{ o.name }}
                  </option>
                </select>

                <div v-if="f.description" class="form-hint">{{ f.description }}</div>
              </div>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-sm" @click="showEdit = false">
            取消
          </button>
          <button
            type="submit"
            class="btn btn-sm btn-primary"
            :disabled="saving"
          >
            保存
          </button>
        </div>
      </form>
    </CdModal>

    <CdModal
      :show="showChangePassword"
      title="修改密码"
      @close="showChangePassword = false"
    >
      <form novalidate @submit.prevent="saveChangePassword">
        <div class="modal-body">
          <p class="text-secondary small mb-3">
            用户：<strong>{{ changePasswordUser?.username }}</strong>
          </p>
          <p class="text-secondary small mb-3">
            直接设置新密码；若需邮件发送随机密码，请使用「更多 → 重置密码」。
          </p>
          <div
            v-if="changePasswordError"
            class="alert alert-danger py-2 small mb-3"
            role="alert"
          >
            {{ changePasswordError }}
          </div>
          <div class="mb-2">
            <label class="form-label required">新密码</label>
            <input
              v-model="changePasswordForm.new_password"
              type="password"
              class="form-control"
              :class="{ 'is-invalid': !!changePasswordError }"
              autocomplete="new-password"
              @input="changePasswordTouched.password = true"
              @blur="changePasswordTouched.password = true"
            />
            <div v-if="changePasswordError" class="invalid-feedback d-block">
              {{ changePasswordError }}
            </div>
            <div v-else class="form-hint">
              密码要求：{{ passwordRulesHint }}
            </div>
          </div>
          <div class="mb-2">
            <label class="form-label required">确认新密码</label>
            <input
              v-model="changePasswordForm.new_password2"
              type="password"
              class="form-control"
              :class="{
                'is-invalid':
                  !!changePasswordError && !!changePasswordForm.new_password,
              }"
              autocomplete="new-password"
              @input="changePasswordTouched.password = true"
              @blur="changePasswordTouched.password = true"
            />
          </div>
        </div>
        <div class="modal-footer">
          <button
            type="button"
            class="btn btn-sm"
            @click="showChangePassword = false"
          >
            取消
          </button>
          <button
            type="submit"
            class="btn btn-sm btn-primary"
            :disabled="changingPassword"
          >
            保存
          </button>
        </div>
      </form>
    </CdModal>

    <!-- 用户详情弹窗（只读 + 编辑入口） -->
    <CdModal
      :show="showDetail"
      :title="`用户详情 - ${detailUser?.username || ''}`"
      size="lg"
      @close="showDetail = false"
    >
      <div class="modal-body">
        <div v-if="detailUser" class="row g-3 mb-3">
          <div class="col-md-6">
            <label class="form-label text-secondary mb-0">用户名</label>
            <div>{{ detailUser.username }}</div>
          </div>
          <div class="col-md-6">
            <label class="form-label text-secondary mb-0">邮箱</label>
            <div>{{ detailUser.email || "—" }}</div>
          </div>
          <div class="col-md-6">
            <label class="form-label text-secondary mb-0">角色</label>
            <div><span class="badge bg-secondary-lt">{{ detailUser.role }}</span></div>
          </div>
          <div class="col-md-6">
            <label class="form-label text-secondary mb-0">部门</label>
            <div>{{ detailUser.department_name || "—" }}</div>
          </div>
          <div class="col-md-6">
            <label class="form-label text-secondary mb-0">状态</label>
            <div>{{ detailUser.disabled ? "禁用" : "正常" }}</div>
          </div>
          <div class="col-md-6">
            <label class="form-label text-secondary mb-0">来源</label>
            <div>
              <span v-if="detailUser.auth_source === 'ldap'" class="badge bg-azure-lt">LDAP</span>
              <span v-else class="badge bg-secondary-lt">本地</span>
            </div>
          </div>
          <div class="col-md-6">
            <label class="form-label text-secondary mb-0">显示名</label>
            <div>{{ detailUser.real_name || "—" }}</div>
          </div>
          <div class="col-md-6">
            <label class="form-label text-secondary mb-0">创建时间</label>
            <div>{{ detailUser.created_at }}</div>
          </div>
        </div>

        <!-- 扩展字段只读展示 -->
        <div v-if="detailCfLoading" class="text-secondary small py-2">加载扩展字段…</div>
        <div v-else-if="detailCfValues.length" class="mt-3 border-top pt-3">
          <h6 class="mb-3 text-secondary">扩展字段</h6>
          <div class="row g-3">
            <div
              v-for="f in detailCfValues"
              :key="f.id"
              class="col-md-6"
            >
              <label class="form-label text-secondary mb-0">{{ f.field_name }}</label>
              <div>{{ detailCfText(f) }}</div>
            </div>
          </div>
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn btn-sm" @click="showDetail = false">关闭</button>
        <button
          class="btn btn-sm btn-primary"
          @click="showDetail = false; openEdit(detailUser!)"
         v-permission="['user:update']">
          编辑
        </button>
      </div>
    </CdModal>
  </div>
</template>
