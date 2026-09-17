import { createApp } from "vue";
import { createPinia } from "pinia";
import { permission } from "./directives/permission";
import App from "./App.vue";
import router from "./router";
import i18n from "./i18n";
import { useBrandingStore } from "./stores/branding";
import { useLicenseStore } from "./stores/license";
import "@tabler/core/dist/css/tabler.min.css";
import "bootstrap/dist/js/bootstrap.bundle.min.js";
import "./styles/app.css";
const app = createApp(App),
  pinia = createPinia();
app.use(pinia).use(i18n).use(router);
app.directive("permission",permission);
app.mount("#app");
void useBrandingStore(pinia).load();
void useLicenseStore(pinia).load();

import "./styles/responsive.css";
