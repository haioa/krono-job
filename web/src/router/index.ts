import { createRouter, createWebHistory } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      component: () => import('@/components/AppLayout.vue'),
      redirect: '/stats',
      children: [
        { path: 'stats', name: 'stats', component: () => import('@/views/StatsView.vue') },
        { path: 'jobs', name: 'jobs', component: () => import('@/views/JobsView.vue') },
        { path: 'logs', name: 'logs', component: () => import('@/views/LogsView.vue') },
        { path: 'logs/:id', name: 'log-detail', component: () => import('@/views/LogDetailView.vue') },
        { path: 'users', name: 'users', component: () => import('@/views/UsersView.vue') },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/stats' },
  ],
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  const isPublic = to.meta.public === true
  if (!isPublic && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'stats' }
  }
  return true
})

export default router
