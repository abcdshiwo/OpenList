package op

import (
	"strconv"

	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/go-cache"
)

var proxyGroupCache = cache.NewMemCache(cache.WithShards[*model.ProxyGroup](2))

func proxyGroupKey(id uint) string { return strconv.Itoa(int(id)) }

func ListProxyGroups() ([]model.ProxyGroup, error) {
	var gs []model.ProxyGroup
	err := db.GetDb().Order("id asc").Find(&gs).Error
	return gs, err
}

func GetProxyGroup(id uint) (*model.ProxyGroup, error) {
	if g, ok := proxyGroupCache.Get(proxyGroupKey(id)); ok && g != nil {
		return g, nil
	}
	var g model.ProxyGroup
	if err := db.GetDb().First(&g, id).Error; err != nil {
		return nil, err
	}
	proxyGroupCache.Set(proxyGroupKey(id), &g)
	return &g, nil
}

func CreateProxyGroup(g *model.ProxyGroup) error {
	if err := db.GetDb().Create(g).Error; err != nil {
		return err
	}
	proxyGroupCache.Set(proxyGroupKey(g.ID), g)
	return nil
}

func UpdateProxyGroup(g *model.ProxyGroup) error {
	if err := db.GetDb().Save(g).Error; err != nil {
		return err
	}
	proxyGroupCache.Set(proxyGroupKey(g.ID), g)
	return nil
}

func DeleteProxyGroup(id uint) error {
	if err := db.GetDb().Delete(&model.ProxyGroup{}, id).Error; err != nil {
		return err
	}
	proxyGroupCache.Del(proxyGroupKey(id))
	return nil
}

// ProxyGroupNodes collects the unique nodes across all groups for health probing.
func ProxyGroupNodes() []string {
	gs, err := ListProxyGroups()
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var nodes []string
	for _, g := range gs {
		for _, n := range g.Nodes {
			if n != "" && !seen[n] {
				seen[n] = true
				nodes = append(nodes, n)
			}
		}
	}
	return nodes
}
