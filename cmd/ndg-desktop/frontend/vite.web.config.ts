import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";
export default defineConfig({ base: "./", plugins: [react()], build: { outDir: "../../../internal/apihttp/webdist", emptyOutDir: true, rollupOptions: { input: resolve(import.meta.dirname, "web.html") } } });
