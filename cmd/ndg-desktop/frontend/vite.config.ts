import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    // Don't empty the directory — the .gitkeep placeholder must survive
    // builds to keep the dist/ directory tracked in git for Go embed.
    emptyOutDir: false,
  },
  server: {
    port: 5173,
    strictPort: true,
  },
});
