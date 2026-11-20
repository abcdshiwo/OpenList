package model

import "time"

// ProxyGroup is an ordered combination of proxy nodes (first = primary,
// rest = backups). Failover happens within the group: link generation picks
// the first healthy node. Different groups serve different scenarios.
type ProxyGroup struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"uniqueIndex;not null"`
	Remark    string    `json:"remark"`
	Nodes     []string  `json:"nodes" gorm:"serializer:json"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (p *ProxyGroup) TableName() string {
	return "x_proxy_groups"
}
