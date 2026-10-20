import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { precompress } from './precompress.ts'

export default defineConfig({
  plugins: [vue(), precompress()],
  build: {
    // The server CSP has font-src 'self', so fonts must never become data: URLs.
    assetsInlineLimit: (filePath) => (/\.(woff2?|ttf|otf|eot)$/i.test(filePath) ? false : undefined)
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8091'
    }
  }
})
