import { createRouter, createWebHistory } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { hasSession } from "@/api/client";
import { rememberLoginRedirect } from "@/utils/authRedirect";
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/login", component: () => import("@/views/LoginView.vue") },
    {
      path: "/",
      component: () => import("@/layouts/MainLayout.vue"),
      meta: { auth: true },
      children: [
        { path: "", redirect: "/admin/overview" },
        {
          path: "profile",
          component: () => import("@/views/ProfileView.vue"),
          meta: { title: "个人中心" },
        },
        {
          path: "admin/overview",
          component: () => import("@/views/admin/OverviewView.vue"),
          meta: { admin: true, title: "工作台" },
        },
        {
          path: "admin/live",
          component: () => import("@/views/admin/LiveMonitorView.vue"),
          meta: { admin: true, title: "实时监控" },
        },
        {
          path: "admin/camera-manage",
          component: () => import("@/views/admin/CameraManageView.vue"),
          meta: { admin: true, title: "视频管理" },
        },
        {
          path: "admin/cameras",
          component: () => import("@/views/admin/CamerasView.vue"),
          meta: { admin: true, title: "监控预览" },
        },
        {
          path: "admin/nvr",
          component: () => import("@/views/admin/NvrAgentsView.vue"),
          meta: { admin: true, title: "NVR 接入" },
        },
        {
          path: "admin/boundaries",
          component: () => import("@/views/admin/BoundariesView.vue"),
          meta: { admin: true, title: "坑边管理" },
        },
        {
          path: "admin/algorithms",
          component: () => import("@/views/admin/AlgorithmsView.vue"),
          meta: { admin: true, title: "算法管理" },
        },
        {
          path: "admin/deployments",
          component: () => import("@/views/admin/DeploymentsView.vue"),
          meta: { admin: true, title: "布控管理" },
        },
        {
          path: "admin/alerts",
          component: () => import("@/views/admin/AlertsView.vue"),
          meta: { admin: true, title: "告警中心" },
        },
        {
          path: "admin/reports",
          component: () => import("@/views/admin/ReportsView.vue"),
          meta: { admin: true, title: "统计报表" },
        },
        {
          path: "admin/access-devices",
          component: () => import("@/views/admin/AccessDevicesView.vue"),
          meta: { admin: true, title: "扩展接入" },
        },
        {
          path: "admin/users",
          component: () => import("@/views/admin/UsersView.vue"),
          meta: { admin: true, title: "用户管理" },
        },
        {
          path: "admin/departments",
          component: () => import("@/views/admin/DepartmentsView.vue"),
          meta: { admin: true, title: "部门管理" },
        },
        {
          path: "admin/roles",
          component: () => import("@/views/admin/RolesView.vue"),
          meta: { admin: true, title: "角色权限" },
        },
        {
          path: "admin/projects",
          component: () => import("@/views/admin/ProjectsView.vue"),
          meta: { admin: true, title: "项目管理" },
        },
        {
          path: "admin/configs",
          component: () => import("@/views/admin/ConfigsView.vue"),
          meta: { admin: true, title: "系统配置" },
        },
        {
          path: "admin/logs",
          component: () => import("@/views/admin/LogsView.vue"),
          meta: { admin: true, title: "系统日志" },
        },
        {
          path: "admin/storages",
          component: () => import("@/views/admin/StoragesView.vue"),
          meta: { admin: true, title: "存储对接" },
        },
        {
          path: "admin/schedules",
          component: () => import("@/views/admin/SchedulesView.vue"),
          meta: { admin: true, title: "计划任务" },
        },
        {
          path: "admin/ops",
          component: () => import("@/views/admin/OpsView.vue"),
          meta: { admin: true, title: "运维中心" },
        },
        {
          path: "admin/dicts",
          component: () => import("@/views/admin/DictsView.vue"),
          meta: { admin: true, title: "字典管理" },
        },
        {
          path: "admin/custom-fields",
          component: () => import("@/views/admin/CustomFieldsView.vue"),
          meta: { admin: true, title: "自定义字段" },
        },
        {
          path: "admin/webhooks",
          component: () => import("@/views/admin/WebhooksView.vue"),
          meta: { admin: true, title: "Webhook" },
        },
        {
          path: "admin/api-call-logs",
          component: () => import("@/views/admin/ApiCallLogsView.vue"),
          meta: { admin: true, title: "API 调用日志" },
        },
      ],
    },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
});
router.beforeEach(async (to) => {
  const auth = useAuthStore();
  if (to.path === "/login") return true;
  if (!hasSession()) {
    rememberLoginRedirect(to.fullPath);
    return "/login";
  }
  try {
    await auth.fetchMe();
  } catch {
    return "/login";
  }
  if (!auth.user) return "/login";
  if ((auth.user.must_change_password || auth.user.must_setup_totp) && to.path !== "/profile") return "/profile";
  if (to.meta.admin && !auth.canPage(to.path)) return "/profile";
});
export default router;
