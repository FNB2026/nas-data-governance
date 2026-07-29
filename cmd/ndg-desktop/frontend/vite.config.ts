import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [
    react(),
    {
      name: "preserve-dist-placeholder",
      generateBundle() {
        this.emitFile({ type: "asset", fileName: ".gitkeep", source: "\n" });
      },
    },
  ],
  build: {
    outDir: "dist",
    // Remove stale hashed assets, then recreate the tracked placeholder above.
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    strictPort: true,
  },
});
