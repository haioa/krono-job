// Package adapter 实现 gRPC「通用」调用适配器（决策 13）。
//
// 不写死任何下游方法：
//   - 通过 request_type（消息全名，如 mall.ProductSyncStockRequest）从 protobuf 全局类型注册表
//     protoregistry.GlobalTypes 取出强类型消息，用 protojson.Unmarshal(yaml payload) 填充参数；
//   - 通过 grpc_service（包.服务）+ grpc_method 拼出完整方法名 /包.服务/方法，用 conn.Invoke 低层通用入口调用；
//   - reply 由方法描述符的输出类型动态构造（dynamicpb），无需在代码里声明。
//
// 新增下游服务：在 internal/worker/pbimports 中加一行 `import _ "downstream/pb"` 触发其
// init 把描述符注册进全局注册表即可，本文件零改动。
package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/haioa/krono-job/internal/scheduler"
)

// Result 是 gRPC 调用的结果（reply 已序列化为 JSON，便于落库与展示）。
type Result struct {
	ReplyJSON string
}

// Invoke 通用调用下游 gRPC 方法。
// conn 由调用方从 grpcpool 取得（按 def.Endpoint 复用）。
func Invoke(ctx context.Context, def scheduler.JobDef, conn *grpc.ClientConn) (*Result, error) {
	// 1) 取请求消息类型（依赖 import _ 注册的描述符）
	reqType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(def.RequestType))
	if err != nil {
		return nil, fmt.Errorf("find request type %q: %w (是否漏加 import _ \"downstream/pb\"?)", def.RequestType, err)
	}
	req := reqType.New().Interface()

	// 2) 把 yaml payload（JSON）反序列化为强类型请求消息
	payloadJSON, err := json.Marshal(def.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	if err := protojson.Unmarshal(payloadJSON, req); err != nil {
		return nil, fmt.Errorf("protojson unmarshal into %q: %w", def.RequestType, err)
	}

	// 3) 由 grpc_service 取服务描述符，再取方法描述符，构造 reply 类型
	svcDesc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(def.GRPCService))
	if err != nil {
		return nil, fmt.Errorf("find service %q: %w", def.GRPCService, err)
	}
	svc, ok := svcDesc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%q is not a service descriptor", def.GRPCService)
	}
	methodDesc := svc.Methods().ByName(protoreflect.Name(def.GRPCMethod))
	if methodDesc == nil {
		return nil, fmt.Errorf("method %q not found in service %q", def.GRPCMethod, def.GRPCService)
	}
	reply := dynamicpb.NewMessage(methodDesc.Output())

	// 4) metadata 经网络传输（ctx.Value 不会发送），并注入系统字段
	md := metadata.New(def.Metadata)
	md.Set("x-task-type", def.TaskType)
	md.Set("x-trace-id", uuid.NewString())
	ctx = metadata.NewOutgoingContext(ctx, md)

	// 5) 通用低层调用 /包.服务/方法
	fullMethod := fmt.Sprintf("/%s/%s", def.GRPCService, def.GRPCMethod)
	if err := conn.Invoke(ctx, fullMethod, req, reply); err != nil {
		return nil, fmt.Errorf("grpc invoke %s: %w", fullMethod, err)
	}

	replyJSON, err := protojson.Marshal(reply)
	if err != nil {
		return nil, fmt.Errorf("marshal reply: %w", err)
	}
	return &Result{ReplyJSON: string(replyJSON)}, nil
}
