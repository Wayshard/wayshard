import { defineConfig } from "vite"
import solid from "vite-plugin-solid"
import tailwindcss from "@tailwindcss/vite"

export default defineConfig({
  plugins: [solid(), tailwindcss()],
  publicDir: false,
  worker: { format: "es" },
  build: { outDir: "dist", emptyOutDir: true },
  server: { port: 5173, proxy: { "/v1": "http://127.0.0.1:7420", "/healthz": "http://127.0.0.1:7420" } },
})
