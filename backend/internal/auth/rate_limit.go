package auth

import (
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type rateClient struct {
	limiter *rate.Limiter
	seen    time.Time
}
type Limiter struct {
	mu      sync.Mutex
	clients map[string]*rateClient
	config  Config
	cleaned time.Time
}

func NewLimiter(c Config) *Limiter { return &Limiter{clients: make(map[string]*rateClient), config: c} }

func (l *Limiter) Allow(ip string, now time.Time) bool {
	// Group IPv6 privacy addresses by /64 rather than allowing trivial address rotation.
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	key := addr.String()
	if addr.Is6() {
		key = netip.PrefixFrom(addr, 64).Masked().String()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.cleaned) >= time.Minute {
		// Expiration must not reset a bucket before it would naturally refill.
		ttl := time.Duration(l.config.RateBurst)*time.Minute/time.Duration(l.config.RatePerMinute) + time.Minute
		if ttl < 10*time.Minute {
			ttl = 10 * time.Minute
		}
		for key, client := range l.clients {
			if now.Sub(client.seen) > ttl {
				delete(l.clients, key)
			}
		}
		l.cleaned = now
	}
	client, exists := l.clients[key]
	if !exists {
		if len(l.clients) >= l.config.RateMaxClients {
			return false
		}
		client = &rateClient{limiter: rate.NewLimiter(rate.Limit(float64(l.config.RatePerMinute)/60), l.config.RateBurst)}
		l.clients[key] = client
	}
	client.seen = now
	return client.limiter.AllowN(now, 1)
}
