// Package pbimports 是「新增下游 gRPC 服务」的唯一改动点（决策 13）。
//
// 通用适配器 adapter 依赖 protobuf 全局类型注册表（protoregistry.GlobalTypes /
// GlobalFiles）。下游 pb 包在 import 时会通过 init 把自己的消息/服务描述符注册进去。
// 因此每接入一个新的下游服务，只需在本文件加一行：
//
//	import _ "your/module/path/to/downstream/pb"
//
// 无需在本项目写任何逐方法对接代码；jobs.yaml 里用 grpc_service / grpc_method /
// request_type 描述调用即可。当前无任何下游 pb 依赖，故为空包。
package pbimports

import _ "gitee.com/haioa/sdc-grpc/pkg/grpc/v1/sdc"
