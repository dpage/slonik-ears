import { writeFileSync } from 'node:fs'
import { join } from 'node:path'
import type { Plugin } from 'vite'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

/**
 * The Go server embeds web/dist with `go:embed`, which needs the directory to
 * contain at least one file even in a checkout where the front end has never
 * been built. A committed .gitkeep does that job, and Vite deletes it every
 * time it empties the output directory — so put it back.
 */
function keepDistTracked(): Plugin {
  return {
    name: 'ears-keep-dist-tracked',
    closeBundle() {
      writeFileSync(join(__dirname, 'dist', '.gitkeep'), '')
    },
  }
}

// During development the web app runs on Vite's own port and proxies the API
// (and the WebSockets) to a locally running ears-server. In production the
// built files are embedded into the server binary and served from the same
// origin, so no proxy is involved.
const backend = process.env.EARS_DEV_SERVER ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react(), keepDistTracked()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Conference Wi-Fi is not a fast pipe. Keep the bundle in one or two
    // files and let the server cache them hard.
    chunkSizeWarningLimit: 700,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: backend, ws: true, changeOrigin: true },
      '/healthz': { target: backend, changeOrigin: true },
    },
  },
})
