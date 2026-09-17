<script setup lang="ts">
import { useAuthStore } from "@/stores/auth";
import { computed } from "vue";
import { RouterLink, useRoute } from "vue-router";
import ConfigSettingsTab from "./config/ConfigSettingsTab.vue";
import ConfigParametersTab from "./config/ConfigParametersTab.vue";

const auth=useAuthStore();
const tabs = [
  { key: "general", label: "基本设置" },
  { key: "appearance", label: "外观品牌" },
  { key: "login", label: "登录页面" },
  { key: "security", label: "安全策略" },
  { key: "email", label: "邮件配置" },
  { key: "ldap", label: "LDAP" },
  { key: "oauth", label: "OAuth" },
  { key: "gateway", label: "网关" },
  { key: "license", label: "授权管理" },
  { key: "watermark", label: "系统水印" },
  { key: "parameters", label: "系统参数" },
];
const visibleTabs=computed(()=>tabs.filter(t=>{
 const key=['appearance','login'].includes(t.key)?'general':t.key;
 if(['security','ldap','oauth','gateway','license'].includes(key)&&!auth.isSuperAdmin)return false;
 return auth.can(key==='parameters'?'config:list':'settings:'+key)
}));

type SettingsTabKey =
  | "general"
  | "appearance"
  | "login"
  | "security"
  | "email"
  | "ldap"
  | "oauth"
  | "gateway"
  | "license"
  | "watermark";

const route = useRoute();

const activeTab = computed(() => {
  const t = String(route.query.tab || "general");
  return (visibleTabs.value.some(x=>x.key===t) ? t : visibleTabs.value[0]?.key || "general") as SettingsTabKey | "parameters";
});
const settingsTab = computed<SettingsTabKey>(() =>
  (activeTab.value === "parameters" ? "general" : activeTab.value) as SettingsTabKey,
);
</script>

<template>
  <div>
    <div class="nav nav-pills mb-4 flex-wrap gap-1">
      <RouterLink
        v-for="t in visibleTabs"
        :key="t.key"
        :to="{ path: '/admin/configs', query: { tab: t.key } }"
        class="nav-link"
        :class="{ active: activeTab === t.key }"
        active-class=""
        exact-active-class=""
      >
        {{ t.label }}
      </RouterLink>
    </div>
    <ConfigParametersTab v-if="activeTab === 'parameters'" />
    <ConfigSettingsTab v-else :tab="settingsTab" />
  </div>
</template>
