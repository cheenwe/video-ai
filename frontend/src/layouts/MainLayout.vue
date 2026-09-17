<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter, RouterLink, RouterView } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { useBrandingStore } from "@/stores/branding";
import { useToast } from "@/composables/useToast";
import { useSidebar } from "@/composables/useSidebar";
import CdAvatar from "@/components/CdAvatar.vue";
import CdModal from "@/components/CdModal.vue";
import WatermarkOverlay from "@/components/WatermarkOverlay.vue";
import {
  IconLayoutDashboard,
  IconUsers,
  IconBuilding,
  IconShieldCheck,
  IconSettings,
  IconListDetails,
  IconLogout,
  IconDatabase,
  IconCalendarTime,
  IconMenu2,
  IconX,
  IconBook,
  IconForms,
  IconBell,
  IconCheck,
  IconWebhook,
  IconApi,
  IconFolder,
  IconChevronDown,
  IconLock,
} from "@tabler/icons-vue";
import { onMounted, onUnmounted, ref, watch } from "vue";
import * as notifApi from "@/api/notification";
import type { Notification } from "@/api/notification";
import * as authApi from "@/api/auth";
import { ApiError, clearToken } from "@/api/client";
const auth = useAuthStore(),
  branding = useBrandingStore(),
  route = useRoute(),
  router = useRouter(),
  toast = useToast(),
  sidebar = useSidebar();
const menuGroups = computed(() => [
  { id: 'workspace', label: '工作空间', icon: IconLayoutDashboard, children: [
    { path: '/admin/overview', label: '工作台', icon: IconLayoutDashboard },
    { path: '/admin/projects', label: '项目管理', icon: IconFolder },
  ] },
  { id: 'organization', label: '组织与权限', icon: IconUsers, children: [
    { path: '/admin/users', label: '用户管理', icon: IconUsers },
    { path: '/admin/departments', label: '部门管理', icon: IconBuilding },
    { path: '/admin/roles', label: '角色权限', icon: IconShieldCheck },
  ] },
  { id: 'platform', label: '平台配置', icon: IconSettings, children: [
    { path: '/admin/configs', label: '系统配置', icon: IconSettings },
    { path: '/admin/dicts', label: '字典管理', icon: IconBook },
    { path: '/admin/custom-fields', label: '自定义字段', icon: IconForms },
    { path: '/admin/storages', label: '存储对接', icon: IconDatabase },
    { path: '/admin/webhooks', label: 'Webhook', icon: IconWebhook },
  ] },
  { id: 'operations', label: '运维与审计', icon: IconListDetails, children: [
    { path: '/admin/ops', label: '运维中心', icon: IconDatabase },
    { path: '/admin/schedules', label: '计划任务', icon: IconCalendarTime },
    { path: '/admin/logs', label: '系统日志', icon: IconListDetails },
    { path: '/admin/api-call-logs', label: 'API 调用日志', icon: IconApi },
  ] },
].map(group => ({ ...group, children: group.children.filter(item => auth.canPage(item.path)) }))
 .filter(group => group.children.length));
const openGroups = ref<string[]>([]);
function toggleGroup(id: string) {
  if (sidebar.collapsed.value && !sidebar.mobileOpen.value) {
    sidebar.expand();
    openGroups.value = [...new Set([...openGroups.value, id])];
  } else {
    openGroups.value = openGroups.value.includes(id) ? openGroups.value.filter(x => x !== id) : [...openGroups.value, id];
  }
}
watch(() => route.path, path => {
  const group = menuGroups.value.find(g => g.children.some(item => item.path === path));
  if (group && !openGroups.value.includes(group.id)) openGroups.value.push(group.id);
}, { immediate: true });
// ---------- 通知中心 ----------
const unreadCount = ref(0);
const showNotifPanel = ref(false);
const notifItems = ref<Notification[]>([]);
const notifLoading = ref(false);

async function refreshUnread() {
  try {
    const res = await notifApi.getUnreadCount();
    unreadCount.value = res.count;
  } catch {
    // ignore
  }
}

async function toggleNotifPanel() {
  showNotifPanel.value = !showNotifPanel.value;
  if (showNotifPanel.value) {
    notifLoading.value = true;
    try {
      const res = await notifApi.listNotifications({ page: 1, page_size: 20 });
      notifItems.value = res.items;
    } catch {
      // ignore
    } finally {
      notifLoading.value = false;
    }
  }
}

async function markAllRead() {
  try {
    await notifApi.markAllNotificationsRead();
    unreadCount.value = 0;
    notifItems.value.forEach((n) => (n.is_read = 1));
  } catch {
    // ignore
  }
}

async function markRead(n: Notification) {
  if (n.is_read) return;
  try {
    await notifApi.markNotificationRead(n.id);
    n.is_read = 1;
    unreadCount.value = Math.max(0, unreadCount.value - 1);
  } catch {
    // ignore
  }
}

let unreadTimer: ReturnType<typeof setInterval>;
onMounted(() => {
  refreshUnread();
  unreadTimer = setInterval(refreshUnread, 60000);
});

// ---------- 折叠侧栏的悬停菜单名提示 ----------
const tooltip = ref<{ label: string; x: number; y: number } | null>(null);

function showTooltip(e: MouseEvent, label: string) {
  if (!sidebar.collapsed.value) return;
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
  tooltip.value = { label, x: rect.right + 12, y: rect.top + rect.height / 2 };
}
function hideTooltip() {
  tooltip.value = null;
}

window.addEventListener('scroll', hideTooltip, true);
window.addEventListener('resize', hideTooltip);

async function logout() {
  try {
    await auth.logout();
    await router.push("/login");
  } catch {
    toast.show("退出请求失败，请重试");
  }
}

// ---------- 用户下拉菜单 ----------
const showUserMenu = ref(false);
const userMenuClose = () => (showUserMenu.value = false);
const userMenuEl = ref<HTMLElement | null>(null);
function onDocClick(e: MouseEvent) {
  const el = userMenuEl.value;
  if (showUserMenu.value && el && !el.contains(e.target as Node)) {
    showUserMenu.value = false;
  }
}
onMounted(() => {
  document.addEventListener("click", onDocClick);
});
// 路由切换后自动关闭移动端抽屉
watch(() => route.fullPath, () => { sidebar.closeMobile(); showNotifPanel.value = false; showUserMenu.value = false; });
function closeOnEscape(e: KeyboardEvent) {
  if (e.key === 'Escape') { sidebar.closeMobile(); showNotifPanel.value = false; showUserMenu.value = false; }
}
const mobileQuery = window.matchMedia('(max-width: 760px)');
const isMobile = ref(mobileQuery.matches);
function onViewportChange() { isMobile.value = mobileQuery.matches; if (!mobileQuery.matches) sidebar.closeMobile(); }
watch(sidebar.mobileOpen, open => document.body.classList.toggle('va-drawer-open', open));
onMounted(() => { document.addEventListener('keydown', closeOnEscape); mobileQuery.addEventListener('change', onViewportChange); });
onUnmounted(() => {
  clearInterval(unreadTimer);
  document.removeEventListener('click', onDocClick);
  document.removeEventListener('keydown', closeOnEscape);
  window.removeEventListener('scroll', hideTooltip, true);
  window.removeEventListener('resize', hideTooltip);
  mobileQuery.removeEventListener('change', onViewportChange);
  document.body.classList.remove('va-drawer-open');
});

// ---------- 首次登录强制修改密码 ----------
const showChangePwdModal = computed(
  () => auth.user?.must_change_password === true || pwdVoluntary.value,
);
const pwdVoluntary = ref(false);
function openPwdModal() {
  pwdVoluntary.value = true;
  showUserMenu.value = false;
}
function closePwdModal() {
  pwdVoluntary.value = false;
  resetPwdForm();
}
function resetPwdForm() {
  pwdForm.value = { old: "", new: "", confirm: "" };
}
const pwdForm = ref({ old: "", new: "", confirm: "" });
const pwdSaving = ref(false);

async function submitChangePwd() {
  if (!pwdForm.value.old || !pwdForm.value.new) {
    toast.show("请填写旧密码和新密码");
    return;
  }
  if (pwdForm.value.new !== pwdForm.value.confirm) {
    toast.show("两次输入的新密码不一致");
    return;
  }
  pwdSaving.value = true;
  try {
    await authApi.changePassword(pwdForm.value.old, pwdForm.value.new);
    if (auth.user) {
      auth.user.must_change_password = false;
    }
    pwdForm.value = { old: "", new: "", confirm: "" };
    toast.show("密码修改成功，请重新登录");
    clearToken(); auth.user=null;
    await router.push("/login");
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "密码修改失败");
  } finally {
    pwdSaving.value = false;
  }
}
</script>
<template>
  <div class="va-shell" :class="{ 'va-collapsed': sidebar.collapsed.value }">
    <aside :inert="isMobile && !sidebar.mobileOpen.value" id="platform-navigation" class="va-sidebar" :class="{ 'va-mobile-open': sidebar.mobileOpen.value }">
      <RouterLink to="/" class="va-brand"
        ><span class="va-mark">VA</span
        ><span class="va-brand-copy"
          >{{ branding.settings.site_name
          }}<small>视频智能分析平台</small></span
        ></RouterLink
      >
      <button class="va-collapse-btn" :title="sidebar.collapsed.value ? '展开' : '收起'" @click="sidebar.toggle">
        <IconMenu2 :size="18" />
      </button>
      <div class="va-section">平台管理</div>
      <button class="va-mobile-close" aria-label="关闭菜单" @click="sidebar.closeMobile()"><IconX :size="20" /></button>
      <nav class="va-navigation" aria-label="主导航">
        <div v-for="group in menuGroups" :key="group.id" class="va-nav-group">
          <button class="va-nav va-nav-parent" :class="{ 'is-active': group.children.some(item => item.path === route.path) }"
            :aria-expanded="(!sidebar.collapsed.value || isMobile) && openGroups.includes(group.id)" :aria-controls="'nav-' + group.id" :aria-label="group.label"
            @click="toggleGroup(group.id)" @mouseenter="e => showTooltip(e, group.label)" @mouseleave="hideTooltip">
            <component :is="group.icon" :size="19" /><span class="va-nav-label">{{ group.label }}</span>
            <IconChevronDown class="va-group-chevron" :class="{ 'is-open': openGroups.includes(group.id) }" :size="16" />
          </button>
          <div v-show="openGroups.includes(group.id)" :id="'nav-' + group.id" class="va-nav-children">
            <RouterLink v-for="item in group.children" :key="item.path" :to="item.path" class="va-nav va-nav-child" @click="sidebar.closeMobile()">
              <component :is="item.icon" :size="17" /><span>{{ item.label }}</span>
            </RouterLink>
          </div>
        </div>
      </nav>
      <div class="va-sidebar-foot">
        <span class="status-dot status-green me-2"></span>基础平台 · v0.1
      </div>
    </aside>
    <div class="va-main" :inert="sidebar.mobileOpen.value">
      <header class="va-header">
        <div class="d-flex align-items-center gap-2 va-header-title">
          <button class="va-burger" aria-label="打开菜单" :aria-expanded="sidebar.mobileOpen.value" aria-controls="platform-navigation" @click="sidebar.toggleMobile()">
            <IconMenu2 :size="20" />
          </button>
          <span class="text-secondary small d-none d-md-inline">管理控制台</span>
          <h2 class="m-0 fs-4 va-header-heading">{{ route.meta.title }}</h2>
        </div>
        <div class="d-flex gap-2 align-items-center">
          <div class="position-relative">
            <button class="va-notif-btn" title="通知" @click="toggleNotifPanel">
              <IconBell :size="18" />
              <span
                v-if="unreadCount > 0"
                class="va-notif-badge position-absolute"
              >
                {{ unreadCount > 99 ? "99+" : unreadCount }}
              </span>
            </button>
            <div
              v-if="showNotifPanel"
              class="va-notif-panel position-absolute end-0 mt-2 card shadow"
              style="width: 360px; max-width: calc(100vw - 24px); z-index: 1050"
            >
              <div class="card-header d-flex justify-content-between align-items-center">
                <strong>通知</strong>
                <button class="btn btn-sm btn-link p-0" @click="markAllRead">全部已读</button>
              </div>
              <div class="card-body p-0" style="max-height: 360px; overflow-y: auto">
                <div v-if="notifLoading" class="text-secondary text-center py-3 small">加载中…</div>
                <div v-else-if="!notifItems.length" class="text-secondary text-center py-4 small">暂无通知</div>
                <div
                  v-for="n in notifItems"
                  :key="n.id"
                  class="va-notif-item p-3 border-bottom"
                  :class="{ 'bg-light': !n.is_read }"
                  @click="markRead(n)"
                >
                  <div class="d-flex justify-content-between">
                    <strong class="small">{{ n.title }}</strong>
                    <IconCheck v-if="n.is_read" :size="14" class="text-success" />
                  </div>
                  <div class="text-secondary small mt-1">{{ n.content }}</div>
                  <div class="text-secondary small mt-1" style="font-size: 11px">
                    {{ new Date(n.created_at || "").toLocaleString() }}
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div class="position-relative" ref="userMenuEl">
            <button
              class="va-user-btn btn btn-sm d-inline-flex align-items-center gap-2"
              :title="auth.displayName"
              @click="showUserMenu = !showUserMenu"
            >
              <CdAvatar :name="auth.displayName" kind="user" size="xs" />
              <span class="va-user-name">{{ auth.displayName }}</span>
              <IconChevronDown :size="14" />
            </button>
            <div
              v-if="showUserMenu"
              class="va-user-menu position-absolute end-0 mt-2 card shadow"
            >
              <div class="va-user-menu-head d-flex align-items-center gap-2 px-3 py-2 border-bottom">
                <CdAvatar :name="auth.displayName" kind="user" size="xs" />
                <div class="va-user-menu-meta lh-1">
                  <div class="fw-semibold small">{{ auth.displayName }}</div>
                  <div class="text-secondary" style="font-size: 11px">{{ auth.isAdmin ? '管理员' : '用户' }}</div>
                </div>
              </div>
              <div class="py-1">
                <RouterLink to="/profile" class="dropdown-item" @click="userMenuClose">
                  <IconSettings :size="15" class="me-2" /> 个人中心
                </RouterLink>
                <button type="button" class="dropdown-item" @click="openPwdModal">
                  <IconLock :size="15" class="me-2" /> 修改密码
                </button>
                <div class="dropdown-divider"></div>
                <button type="button" class="dropdown-item text-danger" @click="logout">
                  <IconLogout :size="15" class="me-2" /> 退出登录
                </button>
              </div>
            </div>
          </div>
        </div>
      </header>
      <main class="va-content"><RouterView /></main>
      <footer class="va-footer">
        {{ branding.settings.site_name }} · 基础管理平台
      </footer>
    </div>
    <Transition name="va-fade">
      <div v-if="sidebar.mobileOpen.value" class="va-scrim" @click="sidebar.closeMobile()"></div>
    </Transition>
    <Transition name="va-tip">
      <div
        v-if="tooltip"
        class="va-nav-tip"
        :style="{ left: tooltip.x + 'px', top: tooltip.y + 'px' }"
      >
        <span class="va-nav-tip-arrow"></span>{{ tooltip.label }}
      </div>
    </Transition>
    <WatermarkOverlay />
    <CdModal :show="showChangePwdModal" :title="pwdVoluntary ? '修改密码' : '首次登录 · 请修改初始密码'" @close="closePwdModal">
      <div class="modal-body">
        <div v-if="!pwdVoluntary" class="alert alert-warning mb-3">
          <strong>安全提醒：</strong>检测到您正在使用系统分配的初始密码登录。为保障账户安全，请在继续操作前修改为新的密码。
        </div>
        <p v-if="!pwdVoluntary" class="text-secondary small mb-3">
          请在「当前密码」中填入初始密码（默认 <code>ChangeMe123</code>，或部署时通过 <code>BASE_ADMIN_PASSWORD</code> 指定的值），然后设置您的新密码。修改成功后需使用新密码重新登录。
        </p>
        <div class="mb-3">
          <label class="form-label">当前密码（初始密码）</label>
          <input v-model="pwdForm.old" type="password" class="form-control" placeholder="请输入当前密码" autocomplete="current-password" />
        </div>
        <div class="mb-3">
          <label class="form-label">新密码</label>
          <input v-model="pwdForm.new" type="password" class="form-control" placeholder="请输入新密码" autocomplete="new-password" />
        </div>
        <div class="mb-3">
          <label class="form-label">确认新密码</label>
          <input v-model="pwdForm.confirm" type="password" class="form-control" placeholder="请再次输入新密码" autocomplete="new-password" />
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn btn-primary" :disabled="pwdSaving" @click="submitChangePwd">
          <span v-if="pwdSaving" class="spinner-border spinner-border-sm me-2"></span>
          确认修改
        </button>
      </div>
    </CdModal>
  </div>
</template>
<style scoped>
.va-shell {
  display: flex;
  min-height: 100vh;
  background: #f5f7fa;
}
.va-sidebar {
  width: 220px;
  position: fixed;
  inset: 0 auto 0 0;
  background: white;
  z-index: 900;
  border-right: 1px solid #e5e9ee;
  display: flex;
  flex-direction: column;
  padding: 22px 12px;
  transition: width 0.2s ease, padding 0.2s ease;
  overflow: hidden;
}
.va-shell.va-collapsed .va-sidebar {
  width: 64px;
  padding-left: 8px;
  padding-right: 8px;
  z-index: 1200;
}
.va-collapse-btn {
  position: absolute;
  top: 30px;
  right: 6px;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  border: 1px solid #e5e9ee;
  background: white;
  color: #627081;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  z-index: 10;
}
.va-collapse-btn:hover {
  color: var(--tblr-primary);
  border-color: var(--tblr-primary);
}
.va-brand {
  display: flex;
  gap: 12px;
  align-items: center;
  text-decoration: none;
  color: #182433;
  font-size: 20px;
  font-weight: 700;
  padding: 0 8px 35px;
  white-space: nowrap;
}
.va-shell.va-collapsed .va-brand {
  justify-content: center;
  padding-left: 0;
  padding-right: 0;
}
.va-shell.va-collapsed .va-brand-copy {
  display: none;
}
.va-brand small {
  display: block;
  font-size: 11px;
  color: #8793a1;
  font-weight: 400;
  margin-top: 4px;
}
.va-mark {
  background: var(--tblr-primary);
  color: white;
  padding: 10px;
  border-radius: 10px;
  font-size: 16px;
  flex-shrink: 0;
}
.va-section {
  padding: 8px 12px;
  font-size: 11px;
  color: #8492a3;
  letter-spacing: 2px;
  white-space: nowrap;
}
.va-shell.va-collapsed .va-section {
  display: none;
}
.va-nav {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  margin: 2px 0;
  color: #627081;
  text-decoration: none;
  border-radius: 8px;
  white-space: nowrap;
}
.va-nav.router-link-active {
  background: #e9f3ee;
  color: var(--tblr-primary);
  font-weight: 600;
}
.va-shell.va-collapsed .va-nav {
  position: relative;
  justify-content: center;
  padding-left: 0;
  padding-right: 0;
}
.va-shell.va-collapsed .va-nav-label {
  display: none;
}
/* 悬停置顶浮层：主题一致浅色 + 置顶 */
.va-nav-tip {
  position: fixed;
  z-index: 5000;
  transform: translateY(-50%);
  background: #fff;
  color: var(--tblr-primary);
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
  padding: 9px 14px;
  border: 1px solid #e5e9ee;
  border-radius: 8px;
  white-space: nowrap;
  box-shadow: 0 7px 20px rgba(24, 36, 51, 0.14), 0 2px 6px rgba(24, 36, 51, 0.08);
  letter-spacing: 0.3px;
}
.va-nav-tip-arrow {
  position: absolute;
  left: -5px;
  top: 50%;
  width: 9px;
  height: 9px;
  margin-top: -5px;
  background: #fff;
  border-left: 1px solid #e5e9ee;
  border-bottom: 1px solid #e5e9ee;
  transform: rotate(45deg);
}
.va-tip-enter-active,
.va-tip-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.va-tip-enter-from,
.va-tip-leave-to {
  opacity: 0;
  transform: translate(4px, -50%);
}
.va-sidebar-foot {
  margin-top: auto;
  font-size: 12px;
  padding: 16px;
  color: #8492a3;
  white-space: nowrap;
}
.va-shell.va-collapsed .va-sidebar-foot {
  display: none;
}
.va-main {
  margin-left: 220px;
  flex: 1;
  min-width: 0;
  transition: margin-left 0.2s ease;
}
.va-shell.va-collapsed .va-main {
  margin-left: 64px;
}
.va-header {
  padding: 10px 24px;
  background: white;
  border-bottom: 1px solid #e5e9ee;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.va-user-btn {
  border: 1px solid #e5e9ee;
  border-radius: 8px;
  padding: 4px 10px 4px 4px;
  background: #fff;
  color: #182433;
  transition: border-color 0.15s, background 0.15s;
}
.va-notif-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 1px solid #e5e9ee;
  border-radius: 8px;
  background: #fff;
  color: #182433;
  position: relative;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s, color 0.15s;
}
.va-notif-btn:hover {
  border-color: var(--tblr-primary);
  color: var(--tblr-primary);
  background: #f4f9f6;
}
.va-notif-badge {
  top: -5px;
  right: -6px;
  min-width: 17px;
  height: 17px;
  padding: 0 4px;
  border-radius: 9px;
  background: #df3f5e;
  border: 2px solid #fff;
  color: #fff;
  font-size: 9px;
  font-weight: 700;
  line-height: 15px;
  text-align: center;
}
.va-notif-btn:hover .va-notif-badge {
  border-color: #fff;
}
.va-user-btn:hover {
  border-color: var(--tblr-primary);
  background: #f4f9f6;
}
.va-user-name {
  font-size: 13px;
  font-weight: 500;
  max-width: 10rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.va-user-menu {
  z-index: 1200;
  width: 172px;
  border-radius: 10px;
  overflow: hidden;
}
.va-user-menu .dropdown-item {
  display: flex;
  align-items: center;
  font-size: 13px;
  padding: 8px 12px;
  cursor: pointer;
  white-space: nowrap;
}
.va-user-menu .dropdown-item:hover {
  background: #f4f9f6;
}
.va-user-menu .dropdown-item.text-danger:hover {
  background: #fef2f2;
}
.va-user-menu-head {
  background: #fafbfc;
}
.va-user-menu-head .va-user-menu-meta {
  min-width: 0;
  text-align: right;
}
.va-user-menu-head .va-user-menu-meta .small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.va-content {
  padding: 30px 32px;
  min-height: 80vh;
}
.va-footer {
  padding: 16px 32px;
  color: #8492a3;
  font-size: 12px;
}
.va-header-title {
  min-width: 0;
}
.va-header-title h2 {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.va-burger {
  display: none;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border: 1px solid #e5e9ee;
  border-radius: 8px;
  background: #fff;
  color: #182433;
  cursor: pointer;
  flex-shrink: 0;
}
.va-burger:hover {
  border-color: var(--tblr-primary);
  color: var(--tblr-primary);
}
.va-scrim {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  z-index: 1200;
}
.va-fade-enter-active,
.va-fade-leave-active {
  transition: opacity 0.2s ease;
}
.va-fade-enter-from,
.va-fade-leave-to {
  opacity: 0;
}
@media (max-width: 760px) {
  .va-shell {
    display: block;
  }
  .va-burger {
    display: inline-flex;
  }
  .va-sidebar {
    position: fixed;
    inset: 0 auto 0 0;
    width: 280px;
    padding: 22px 12px;
    z-index: 1300;
    transform: translateX(-100%);
    transition: transform 0.25s ease;
    overflow-y: auto;
    border-right: 1px solid #e5e9ee;
    box-shadow: 8px 0 24px rgba(15, 23, 42, 0.12);
  }
  .va-sidebar.va-mobile-open {
    transform: translateX(0);
  }
  .va-shell.va-collapsed .va-sidebar {
    width: 280px;
    padding-left: 12px;
    padding-right: 12px;
  }
  .va-shell.va-collapsed .va-brand {
    justify-content: flex-start;
    padding-left: 8px;
    padding-right: 8px;
  }
  .va-shell.va-collapsed .va-brand-copy,
  .va-shell.va-collapsed .va-nav-label,
  .va-shell.va-collapsed .va-section,
  .va-shell.va-collapsed .va-sidebar-foot {
    display: revert;
  }
  .va-shell.va-collapsed .va-nav {
    justify-content: flex-start;
    padding-left: 12px;
    padding-right: 12px;
  }
  .va-collapse-btn {
    display: none;
  }
  .va-nav-tip {
    display: none;
  }
  .va-main {
    margin: 0;
  }
  .va-header {
    padding: 10px 14px;
  }
  .va-user-name {
    display: none;
  }
  .va-content {
    padding: 18px 14px;
  }
  .va-footer {
    padding: 14px;
  }
}

.va-navigation { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; }
.va-sidebar-foot { flex-shrink: 0; }
.va-brand-copy { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
.va-nav-parent { border: 0; background: transparent; width: 100%; text-align: left; cursor: pointer; }
.va-nav-parent.is-active { color: var(--tblr-primary); font-weight: 600; }
.va-group-chevron { margin-left: auto; flex-shrink: 0; transition: transform .2s; transform: rotate(-90deg); }
.va-group-chevron.is-open { transform: rotate(0); }
.va-nav-child { font-size: 13px; padding-left: 30px; }
.va-mobile-close { display: none; }
.va-shell.va-collapsed .va-nav-children, .va-shell.va-collapsed .va-group-chevron { display: none; }
@media (max-width: 760px) {
  .va-shell.va-collapsed .va-main { margin-left: 0; }
  .va-shell.va-collapsed .va-sidebar { z-index: 1300; }
  .va-sidebar { width: min(300px, calc(100vw - 40px)); height: 100dvh; }
  .va-shell.va-collapsed .va-sidebar { width: min(300px, calc(100vw - 40px)); }
  .va-mobile-close { display: grid; place-items: center; position: absolute; right: 8px; top: 8px; width: 40px; height: 40px; border: 0; border-radius: 8px; background: #f5f7fa; }
  .va-shell.va-collapsed .va-nav-children { display: block; }
  .va-shell.va-collapsed .va-group-chevron { display: block; }
  .va-shell.va-collapsed .va-nav-child { padding-left: 30px; }
  .va-nav { min-height: 44px; }
  .va-notif-btn, .va-burger, .va-user-btn { min-width: 44px; min-height: 44px; }
  .va-notif-panel { position: fixed !important; left: 12px; right: 12px !important; top: 64px; width: auto !important; max-width: none !important; }
  .va-notif-panel .card-body { max-height: min(360px, calc(100dvh - 140px)) !important; }
  .va-header { gap: 8px; }
  .va-header-title { flex: 1; }
  .va-footer { overflow-wrap: anywhere; }
}
</style>
