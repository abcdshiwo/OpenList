package proxyhealth

import (
	"testing"
	"time"
)

func TestPickHealthyFailoverAndFailback(t *testing.T) {
	c := New(time.Second, time.Second, 2)
	nodes := []string{"https://a.example:6001", "https://b.example:6001"}
	if got := c.PickHealthy(nodes); got != nodes[0] {
		t.Fatalf("unknown nodes should pick primary, got %s", got)
	}
	c.mu.Lock()
	c.state[nodes[0]] = &nodeState{fails: 2, healthy: false}
	c.mu.Unlock()
	if got := c.PickHealthy(nodes); got != nodes[1] {
		t.Fatalf("primary down should failover to backup, got %s", got)
	}
	c.mu.Lock()
	c.state[nodes[1]] = &nodeState{fails: 2, healthy: false}
	c.mu.Unlock()
	if got := c.PickHealthy(nodes); got != nodes[0] {
		t.Fatalf("all down should fall back to first, got %s", got)
	}
	if c.PickHealthy(nil) != "" {
		t.Fatal("empty list should return empty")
	}
	c.mu.Lock()
	c.state[nodes[0]] = &nodeState{healthy: true}
	c.mu.Unlock()
	if got := c.PickHealthy(nodes); got != nodes[0] {
		t.Fatalf("primary recovered should failback, got %s", got)
	}
}
