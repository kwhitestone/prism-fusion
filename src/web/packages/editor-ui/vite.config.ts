import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { fileURLToPath } from "node:url";
export default defineConfig({
  root: fileURLToPath(new URL(".", import.meta.url)),
  plugins: [vue()],
  build: {
    lib: { entry: fileURLToPath(new URL("src/index.ts", import.meta.url)), formats: ["es"], fileName: () => "editor.js", cssFileName: "editor" },
    rollupOptions: { external: ["vue"] },
    sourcemap: false,
  },
});
