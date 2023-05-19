package serverpool

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"
)

const (
	healthLoopDurationHealthy   = 6 * time.Second
	healthLoopDurationUnHealthy = 1 * time.Second
)

type Healthchecker interface {
	Healthcheck(specificServer *net.UDPAddr) error
}

// Pool is a UDP server pool with healthchecking
type Pool struct {
	sync.Mutex
	checker   Healthchecker
	servers   []*net.UDPAddr
	healthy   []*net.UDPAddr
	unhealthy []*net.UDPAddr
	disposed  bool
	Debug     bool
}

func NewPool(getChecker Healthchecker, servers []*net.UDPAddr) *Pool {
	p := &Pool{
		checker: getChecker,
		servers: servers,
	}
	return p
}

func (p *Pool) Listen() {
	// seed health and unhealthy servers immediately
	healthy, unhealthy := p.healthcheck()
	p.Lock()
	p.healthy = healthy
	p.unhealthy = unhealthy
	p.Unlock()

	go p.loopHealthcheck()
}

// healthcheck is slow and should not block the main thread
func (p *Pool) loopHealthcheck() {
	if p.disposed {
		return
	}
	healthy, unhealthy := p.healthcheck()
	p.Lock()
	p.healthy = healthy
	p.unhealthy = unhealthy
	p.Unlock()

	if len(healthy) > 0 {
		time.Sleep(healthLoopDurationHealthy)
	} else {
		time.Sleep(healthLoopDurationUnHealthy)
	}
	p.loopHealthcheck()
}

func (p *Pool) healthcheck() (healthy, unhealthy []*net.UDPAddr) {
	var err error
	for _, s := range p.servers {
		err = p.checker.Healthcheck(s)
		if err != nil {
			unhealthy = append(unhealthy, s)
			if p.Debug {
				fmt.Println("dracula pool server unhealthy", s, err)
			}
		} else {
			healthy = append(healthy, s)
		}
	}
	return healthy, unhealthy
}

func (p *Pool) Choose() *net.UDPAddr {
	p.Lock()
	defer p.Unlock()
	l := len(p.healthy)
	if l < 1 {
		return nil
	}
	if l == 1 {
		return p.healthy[0]
	}
	ix := rand.Intn(l)
	return p.healthy[ix]
}

func (p *Pool) ListServers() string {
	return fmt.Sprintf("%v", p.servers)
}

func (p *Pool) ListHealthy() string {
	return fmt.Sprintf("%v", p.healthy)
}
func (p *Pool) ListUnHealthy() string {
	return fmt.Sprintf("%v", p.unhealthy)
}

func (p *Pool) Dispose() {
	p.disposed = true
}
