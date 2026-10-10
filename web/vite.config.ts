import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import vueDevTools from 'vite-plugin-vue-devtools'

// 部署在 https://<域名>/krono-job/ 下（由 web-go-admin 的 nginx 反向代理，见其 nginx.conf）。
// 必须是子路径 base：这样打包出的资源引用为 /krono-job/assets/xxx.js，
// import.meta.env.BASE_URL 也随之变成 /krono-job/，vue-router 的 history base 会自动跟随，
// 否则页面里的 /assets、/api 会打到站点根路径，被 nginx 的 location / 判为 404。
// 独立部署（根路径 /）时把 base 改回 '/' 即可。
const BASE = '/krono-job/'

// https://vite.dev/config/
export default defineConfig({
  base: BASE,
  plugins: [vue(), vueJsx(), vueDevTools()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    // 开发期将 /krono-job/api 代理到后端服务（默认 10010），避免跨域。
    // 路径与生产一致（生产由 nginx 剥掉 /krono-job 前缀后交给后端），这里同样剥掉再转发。
    proxy: {
      [BASE + 'api']: {
        target: process.env.VITE_API_TARGET || 'http://localhost:10010',
        changeOrigin: true,
        rewrite: (p) => p.replace(new RegExp('^' + BASE + 'api'), '/api'),
      },
    },
  },
})
