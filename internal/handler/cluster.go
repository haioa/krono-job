package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/haioa/krono-job/internal/cluster"
)

// ClusterHandler 暴露分布式集群运行状态：下游节点存活探测结果与选主状态。
// 注意：下游节点存活由调用方主动探测得到（cluster.NodeHealth），下游不注册心跳。
type ClusterHandler struct {
	health      *cluster.NodeHealth
	leader      *cluster.Leader
	nodeID      string
	distributed bool
}

// NewClusterHandler 构造集群状态 Handler。health/leader 在非分布式模式下为 nil。
func NewClusterHandler(health *cluster.NodeHealth, leader *cluster.Leader, nodeID string, distributed bool) *ClusterHandler {
	return &ClusterHandler{health: health, leader: leader, nodeID: nodeID, distributed: distributed}
}

// Nodes 处理 GET /api/cluster/nodes，返回调用方探测得到的下游节点健康视图。
func (h *ClusterHandler) Nodes(c *gin.Context) {
	if !h.distributed || h.health == nil {
		c.JSON(http.StatusOK, gin.H{"distributed": false, "nodes": []any{}})
		return
	}
	views := h.health.Snapshot(c.Request.Context())
	out := make([]gin.H, 0, len(views))
	for _, v := range views {
		out = append(out, gin.H{
			"endpoint": v.Endpoint,
			"protocol": v.Protocol,
			"alive":    v.Alive,
			"error":    v.Error,
			"checked":  v.Checked,
			"by":       v.By,
		})
	}
	c.JSON(http.StatusOK, gin.H{"distributed": true, "nodes": out})
}

// Status 处理 GET /api/cluster/status，返回本节点选主角色。
func (h *ClusterHandler) Status(c *gin.Context) {
	if !h.distributed || h.leader == nil {
		c.JSON(http.StatusOK, gin.H{
			"distributed": false,
			"node_id":     h.nodeID,
			"role":        "standalone",
		})
		return
	}
	role := "follower"
	if h.leader.IsLeader() {
		role = "leader"
	}
	c.JSON(http.StatusOK, gin.H{
		"distributed": true,
		"node_id":     h.nodeID,
		"role":        role,
		"is_leader":   h.leader.IsLeader(),
	})
}
