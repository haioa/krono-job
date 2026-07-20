package web

import "embed"

// Dist 包含前端构建产物 web/dist（由 web/ 下的 `pnpm build` 生成）。
// 经 go:embed 打包进二进制，使平台成为「单二进制交付」。
// 若尚未构建，dist 仅含占位 index.html，运行期会提示先构建前端。
//
//go:embed dist
var Dist embed.FS
