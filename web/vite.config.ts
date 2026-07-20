import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import vueDevTools from 'vite-plugin-vue-devtools'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue(), vueJsx(), vueDevTools()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    // 开发期将 /api 代理到后端服务（默认 10010），避免跨域、与打包后同源行为一致。
    proxy: {
      '/api': {
        target: process.env.VITE_API_TARGET || 'http://localhost:10010',
        changeOrigin: true,
      },
    },
  },
})
