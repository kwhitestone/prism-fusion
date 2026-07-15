import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import legacy from "@vitejs/plugin-legacy";
import vueDevTools from "vite-plugin-vue-devtools";
import UnoCSS from "@unocss/vite";
import * as path from "path";
import * as fs from "fs";
import * as dotenv from "dotenv";

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
  // 加载环境变量
  const envFiles = [`.env.${mode}`];
  for (const file of envFiles) {
    if (fs.existsSync(file)) {
      const envConfig = dotenv.parse(fs.readFileSync(file));
      for (const k in envConfig) {
        process.env[k] = envConfig[k];
      }
    }
  }

  return {
    base: "/",
    root: "./",
    publicDir: "public",
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
        vue$: "vue/dist/vue.runtime.esm-bundler.js"
      }
    },
    define: {
      "process.env": {}
    },
    css: {
      preprocessorOptions: {
        scss: {
          api: "modern-compiler"
        }
      }
    },
    server: {
      open: true,
      port: process.env.VITE_CLI_PORT || 8889,
      proxy: {
        "/api": {
          target: process.env.VITE_API_BASE_URL || "http://localhost:3380",
          changeOrigin: true
          // 保持原始路径 /api/v1/xxx，不需要重写
        }
      }
    },
    build: {
      minify: "terser",
      manifest: false,
      sourcemap: false,
      outDir: "dist",
      terserOptions: {
        compress: {
          drop_console: true,
          drop_debugger: true
        }
      },
      rollupOptions: {
        output: {
          entryFileNames: "assets/[name].[hash].js",
          chunkFileNames: "assets/[name].[hash].js",
          assetFileNames: "assets/[name].[hash].[ext]"
        }
      }
    },
    optimizeDeps: {
      include: ["vue", "vue-router", "pinia", "axios", "element-plus"]
    },
    plugins: [
      vue(),
      legacy({
        targets: [
          "Android > 39",
          "Chrome >= 60",
          "Safari >= 10.1",
          "iOS >= 10.3",
          "Firefox >= 54",
          "Edge >= 15"
        ]
      }),
      process.env.NODE_ENV === "development" && vueDevTools(),
      UnoCSS()
    ].filter(Boolean)
  };
});
