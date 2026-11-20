package handles

import (
	"strconv"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/internal/proxyhealth"
	"github.com/OpenListTeam/OpenList/v4/server/common"
	"github.com/gin-gonic/gin"
)

func ListProxyGroups(c *gin.Context) {
	gs, err := op.ListProxyGroups()
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, gs)
}

func GetProxyGroup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Query("id"))
	g, err := op.GetProxyGroup(uint(id))
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, g)
}

func CreateProxyGroup(c *gin.Context) {
	var g model.ProxyGroup
	if err := c.ShouldBind(&g); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if g.Name == "" {
		common.ErrorStrResp(c, "name required", 400)
		return
	}
	if err := op.CreateProxyGroup(&g); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, g)
}

func UpdateProxyGroup(c *gin.Context) {
	var g model.ProxyGroup
	if err := c.ShouldBind(&g); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := op.UpdateProxyGroup(&g); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, g)
}

func DeleteProxyGroup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Query("id"))
	if err := op.DeleteProxyGroup(uint(id)); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}

// ProxyGroupHealth reports per-node health for the management UI.
func ProxyGroupHealth(c *gin.Context) {
	nodes := op.ProxyGroupNodes()
	type st struct {
		Node    string `json:"node"`
		Healthy bool   `json:"healthy"`
		Known   bool   `json:"known"`
	}
	out := make([]st, 0, len(nodes))
	for _, n := range nodes {
		hh, k := proxyhealth.Default.Status(n)
		out = append(out, st{Node: n, Healthy: hh, Known: k})
	}
	common.SuccessResp(c, out)
}
