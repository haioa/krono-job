// 生成唯一任务标识：UUID + 时间戳，保证不重复且符合「字母、数字、下划线」规则。
// 浏览器原生 crypto.randomUUID 生成标准 UUID（含连字符），这里去掉连字符并拼上毫秒时间戳。

export function genTaskId(prefix = 'job'): string {
  const uuid =
    typeof crypto !== 'undefined' && 'randomUUID' in crypto
      ? crypto.randomUUID().replace(/-/g, '')
      : Math.random().toString(16).slice(2) + Date.now().toString(16)
  const ts = Date.now().toString()
  return `${prefix}_${uuid}_${ts}`
}
