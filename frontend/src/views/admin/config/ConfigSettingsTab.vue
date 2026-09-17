<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { getSettings } from "@/api/settings";
import { useToast } from "@/composables/useToast";
import GeneralPanel from "./GeneralPanel.vue";
import SecurityPanel from "./SecurityPanel.vue";
import EmailPanel from "./EmailPanel.vue";
import LDAPPanel from "./LDAPPanel.vue";
import OAuthPanel from "./OAuthPanel.vue";
import GatewayPanel from "./GatewayPanel.vue";
import AppearancePanel from "./AppearancePanel.vue";
import LoginSettingsPanel from "@/components/admin/LoginSettingsPanel.vue";
import LicensePanel from "./LicensePanel.vue";
import WatermarkPanel from "./WatermarkPanel.vue";

type TabKey =
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

const props = defineProps<{ tab: TabKey }>();
const toast = useToast();

// 需要 v-model（外部加载配置）的面板：tab → settingKey
const settingsTabs: Record<string, string> = {
  general: "general",
  security: "security",
  email: "email",
  ldap: "ldap",
  oauth: "oauth",
  gateway: "gateway",
};

const data = ref<any>(null);
const loading = ref(true);

const settingKey = computed(() => settingsTabs[props.tab]);

async function loadData() {
  loading.value = true;
  data.value = null;
  const key = settingKey.value;
  if (key) {
    try {
      data.value = await getSettings<any>(key);
    } catch (e) {
      toast.show(String(e));
    }
  }
  loading.value = false;
}

onMounted(loadData);
watch(() => props.tab, loadData);
</script>

<template>
  <div class="card">
    <div class="card-body">
      <div v-if="loading" class="text-secondary text-center py-5">加载中…</div>
      <GeneralPanel v-else-if="tab === 'general' && data" v-model="data" />
      <AppearancePanel v-else-if="tab === 'appearance'" />
      <LoginSettingsPanel v-else-if="tab === 'login'" />
      <SecurityPanel v-else-if="tab === 'security' && data" v-model="data" />
      <EmailPanel v-else-if="tab === 'email' && data" v-model="data" />
      <LDAPPanel v-else-if="tab === 'ldap' && data" v-model="data" />
      <OAuthPanel v-else-if="tab === 'oauth' && data" v-model="data" />
      <GatewayPanel v-else-if="tab === 'gateway' && data" v-model="data" />
      <LicensePanel v-else-if="tab === 'license'" />
      <WatermarkPanel v-else-if="tab === 'watermark'" />
    </div>
  </div>
</template>
