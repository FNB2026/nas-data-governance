import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";
export default defineConfig({ plugins: [react()], build: { outDir: "../../../internal/apihttp/webdist", emptyOutDir: false, rollupOptions: { input: resolve(import.meta.dirname, "web.html") } } });
