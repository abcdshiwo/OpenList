package proxyhealth

import (
	"crypto/tls"
	"net"
	"net/url"
	"sync"
	"time"
)

type nodeState struct {
	fails   int
	healthy bool
	last    time.Time
}

// Checker probes proxy nodes for reachability (TCP/TLS dial) and tracks
// health. A node is marked down after FailThreshold consecutive failures
// and recovers on the first successful probe. Unknown nodes are healthy.
type Checker struct {
	mu            sync.RWMutex
	state         map[string]*nodeState
	Interval      time.Duration
	Timeout       time.Duration
	FailThreshold int
}

func New(interval, timeout time.Duration, failThreshold int) *Checker {
	if failThreshold < 1 {
		failThreshold = 1
	}
	return &Checker{
		state:         make(map[string]*nodeState),
		Interval:      interval,
		Timeout:       timeout,
		FailThreshold: failThreshold,
	}
}

// Default is the process-wide checker used by link generation.
var Default = New(15*time.Second, 3*time.Second, 2)

func (c *Checker) Healthy(rawurl string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.state[rawurl]
	if !ok {
		return true
	}
	return s.healthy
}

// Status returns (healthy, known) for UI display.
func (c *Checker) Status(rawurl string) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.state[rawurl]
	if !ok {
		return true, false
	}
	return s.healthy, true
}

func (c *Checker) probe(rawurl string) {
	u, err := url.Parse(rawurl)
	if err != nil || u.Hostname() == "" {
		return
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	addr := net.JoinHostPort(u.Hostname(), port)
	d := &net.Dialer{Timeout: c.Timeout}
	var conn net.Conn
	if u.Scheme == "https" {
		conn, err = tls.DialWithDialer(d, "tcp", addr, &tls.Config{InsecureSkipVerify: true})
	} else {
		conn, err = d.Dial("tcp", addr)
	}
	ok := err == nil
	// Only close a successfully established connection; a failed dial may
	// return a partially initialized conn whose Close panics.
	if ok && conn != nil {
		_ = conn.Close()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.state[rawurl]
	if s == nil {
		s = &nodeState{healthy: true}
		c.state[rawurl] = s
	}
	if ok {
		s.fails = 0
		s.healthy = true
	} else {
		s.fails++
		if s.fails >= c.FailThreshold {
			s.healthy = false
		}
	}
	s.last = time.Now()
}

// CheckOnce probes every node once.
func (c *Checker) CheckOnce(nodes []string) {
	for _, n := range nodes {
		c.probe(n)
	}
}

// Run loops forever, refreshing the node set via getNodes each interval.
func (c *Checker) Run(getNodes func() []string) {
	c.CheckOnce(getNodes())
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	for range t.C {
		c.CheckOnce(getNodes())
	}
}

// PickHealthy returns the first healthy node, falling back to the first node
// when none are healthy (best effort), or "" for an empty list.
func (c *Checker) PickHealthy(nodes []string) string {
	for _, n := range nodes {
		if c.Healthy(n) {
			return n
		}
	}
	if len(nodes) > 0 {
		return nodes[0]
	}
	return ""
}
