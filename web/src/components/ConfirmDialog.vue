<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'

const props = withDefaults(
  defineProps<{
    open?: boolean
    title?: string
    message?: string
    confirmText?: string
    cancelText?: string
    danger?: boolean
    loading?: boolean
  }>(),
  {
    open: false,
    title: '提示',
    message: '',
    confirmText: '确定',
    cancelText: '取消',
    danger: false,
    loading: false,
  },
)

const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()

const dialogRef = ref<HTMLElement | null>(null)

function close() {
  if (props.loading) return // 提交中禁止关闭，避免误操作
  emit('update:open', false)
  emit('cancel')
}

function onConfirm() {
  if (props.loading) return
  emit('confirm')
}

function onKeydown(e: KeyboardEvent) {
  if (!props.open) return
  if (e.key === 'Escape') {
    e.preventDefault()
    close()
  }
}

watch(
  () => props.open,
  (v) => {
    if (v) {
      window.addEventListener('keydown', onKeydown)
      // 打开时让对话框获焦，便于键盘（Esc）操作。
      requestAnimationFrame(() => dialogRef.value?.focus())
    } else {
      window.removeEventListener('keydown', onKeydown)
    }
  },
)

onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <Transition name="cd-fade">
      <div v-if="open" class="cd-overlay" @click.self="close">
        <div
          ref="dialogRef"
          class="cd-modal"
          :class="{ danger }"
          role="alertdialog"
          aria-modal="true"
          tabindex="-1"
        >
          <header class="cd-head">
            <span class="cd-icon" :class="{ danger }">
              <svg viewBox="0 0 24 24" width="20" height="20">
                <path
                  v-if="danger"
                  d="M12 8v5M12 16.5v.5"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                />
                <path
                  v-if="danger"
                  d="M12 3l9 16H3z"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="1.8"
                  stroke-linejoin="round"
                />
                <circle v-else cx="12" cy="12" r="9" fill="none" stroke="currentColor" stroke-width="1.8" />
                <path v-if="!danger" d="M12 8v5M12 16.5v.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
              </svg>
            </span>
            <h3 class="cd-title">{{ title }}</h3>
            <button class="cd-close" type="button" :disabled="loading" @click="close" aria-label="关闭">
              <svg viewBox="0 0 20 20" width="16" height="16"><path d="M5 5l10 10M15 5L5 15" stroke="currentColor" stroke-width="2" stroke-linecap="round" /></svg>
            </button>
          </header>
          <div class="cd-body">
            <p class="cd-msg">{{ message }}</p>
          </div>
          <footer class="cd-foot">
            <button class="cd-btn cd-btn-ghost" type="button" :disabled="loading" @click="close">
              {{ cancelText }}
            </button>
            <button
              class="cd-btn"
              :class="danger ? 'cd-btn-danger' : 'cd-btn-primary'"
              type="button"
              :disabled="loading"
              @click="onConfirm"
            >
              <svg v-if="loading" class="cd-spin" viewBox="0 0 24 24" width="15" height="15">
                <path d="M12 3a9 9 0 1 0 9 9" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" />
              </svg>
              {{ loading ? '处理中…' : confirmText }}
            </button>
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.cd-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: rgba(17, 22, 38, 0.45);
  backdrop-filter: blur(2px);
}
.cd-modal {
  width: 100%;
  max-width: 420px;
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-md);
  outline: none;
  overflow: hidden;
}
.cd-modal:focus-visible {
  box-shadow: var(--shadow-md), 0 0 0 3px var(--primary-ring);
}
.cd-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 18px 18px 6px;
}
.cd-icon {
  display: inline-flex;
  color: var(--primary);
  flex: 0 0 auto;
}
.cd-icon.danger {
  color: var(--danger);
}
.cd-title {
  margin: 0;
  flex: 1 1 auto;
  font-size: 15px;
  font-weight: 700;
  color: var(--text);
}
.cd-close {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--muted);
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}
.cd-close:hover:not(:disabled) {
  background: var(--bg);
  color: var(--text-2);
}
.cd-body {
  padding: 4px 18px 18px;
}
.cd-msg {
  margin: 0;
  font-size: 13.5px;
  line-height: 1.6;
  color: var(--text-2);
  white-space: pre-line;
}
.cd-foot {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 18px;
  border-top: 1px solid var(--border);
  background: var(--panel-2);
}
.cd-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 36px;
  padding: 0 16px;
  font-size: 13px;
  font-weight: 600;
  border-radius: var(--radius-sm);
  border: 1px solid transparent;
  cursor: pointer;
  transition: background 0.15s ease, opacity 0.15s ease, border-color 0.15s ease;
}
.cd-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.cd-btn-ghost {
  background: transparent;
  color: var(--text-2);
  border-color: var(--border-d);
}
.cd-btn-ghost:hover:not(:disabled) {
  background: var(--bg);
}
.cd-btn-primary {
  background: var(--primary);
  color: #fff;
}
.cd-btn-primary:hover:not(:disabled) {
  background: var(--primary-d);
}
.cd-btn-danger {
  background: var(--danger);
  color: #fff;
}
.cd-btn-danger:hover:not(:disabled) {
  opacity: 0.9;
}
.cd-spin {
  animation: cd-rotate 0.8s linear infinite;
}
@keyframes cd-rotate {
  to {
    transform: rotate(360deg);
  }
}

/* 过渡动画 */
.cd-fade-enter-active,
.cd-fade-leave-active {
  transition: opacity 0.18s ease;
}
.cd-fade-enter-from,
.cd-fade-leave-to {
  opacity: 0;
}
.cd-fade-enter-active .cd-modal,
.cd-fade-leave-active .cd-modal {
  transition: transform 0.18s ease, opacity 0.18s ease;
}
.cd-fade-enter-from .cd-modal,
.cd-fade-leave-to .cd-modal {
  transform: translateY(8px) scale(0.98);
  opacity: 0;
}
</style>
