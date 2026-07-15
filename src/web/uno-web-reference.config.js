import { defineConfig, presetWind } from "unocss";
import transformerDirectives from "@unocss/transformer-directives";

export default defineConfig({
  presets: [presetWind()],
  transformers: [transformerDirectives()],
  theme: {
    colors: {
      primary: "#409EFF",
      success: "#67C23A",
      warning: "#E6A23C",
      danger: "#F56C6C",
      info: "#909399"
    }
  }
});
