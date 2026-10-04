import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Relative base so the app works under any path prefix (Coder subdomain or path apps).
export default defineConfig({
  base: "./",
  plugins: [react()],
  server: {
    proxy: {
      // `npm run dev` talks to a locally running stay daemon.
      "/ws": { target: "ws://127.0.0.1:7681", ws: true },
    },
  },
});
