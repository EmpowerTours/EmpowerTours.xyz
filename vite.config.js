import { defineConfig } from "vite";
import { resolve } from "node:path";

export default defineConfig({
  build: {
    target: "es2020",
    rollupOptions: {
      input: { main: resolve(import.meta.dirname, "index.html"), privacy: resolve(import.meta.dirname, "privacy.html"), start: resolve(import.meta.dirname, "start.html") },
    },
  },
});
