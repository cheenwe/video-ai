import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { fileURLToPath, URL } from "node:url";
import { tablerIconsDeepImport } from "./vite-plugin-tabler-icons";
export default defineConfig({
  plugins: [tablerIconsDeepImport(), vue()],
  define: { __APP_VERSION__: JSON.stringify("0.1.0") },
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": {
        target: process.env.BASE_API_TARGET || "http://127.0.0.1:8181",
        changeOrigin: false,
      },
    },
  },
});
