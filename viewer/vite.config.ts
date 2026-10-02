import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In development the API runs separately: `itos-cc serve` on port 7070.
const api = process.env.ITOS_CC_API ?? "http://127.0.0.1:7070";

export default defineConfig({
  plugins: [react()],
  server: { proxy: { "/api": { target: api, changeOrigin: true } } },
});
