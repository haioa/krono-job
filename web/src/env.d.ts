/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** API 基址，默认 /api（dev 经 Vite 代理，生产随二进制同源）。 */
  readonly VITE_API_BASE?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
