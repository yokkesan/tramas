package auth_test

import (
	"net"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"tramas/backend/internal/auth"

	"github.com/labstack/echo/v5"
)

func TestRateLimitRefillIsolationAndCapacity(t *testing.T) {
	c := auth.DefaultConfig()
	c.RateMaxClients = 2
	l := auth.NewLimiter(c)
	now := time.Now()
	if !l.Allow("192.0.2.1", now) || !l.Allow("192.0.2.1", now) || l.Allow("192.0.2.1", now) {
		t.Fatal("burst not enforced")
	}
	if !l.Allow("192.0.2.1", now.Add(12*time.Second)) {
		t.Fatal("no refill")
	}
	if !l.Allow("192.0.2.2", now) || l.Allow("192.0.2.3", now) {
		t.Fatal("client capacity/isolation failed")
	}
	if !l.Allow("192.0.2.3", now.Add(11*time.Minute)) {
		t.Fatal("expired client entries not removed")
	}
	if l.Allow("not-an-ip", now) {
		t.Fatal("invalid IP accepted")
	}
}

func TestRateLimiterConcurrentAndIPv6(t *testing.T) {
	l := auth.NewLimiter(auth.DefaultConfig())
	now := time.Now()
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow("2001:db8::1", now) {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 2 || l.Allow("2001:db8::2", now) {
		t.Fatal("concurrent/IPv6 limit bypassed")
	}
}

func TestProxyTrust(t *testing.T) {
	_, network, _ := net.ParseCIDR("10.93.17.2/32")
	extractor := echo.ExtractIPFromXFFHeader(echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false), echo.TrustIPRange(network))
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "192.0.2.5:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	if extractor(r) != "192.0.2.5" {
		t.Fatal("untrusted forwarding header accepted")
	}
	r.RemoteAddr = "10.93.17.2:1234"
	if extractor(r) != "198.51.100.7" {
		t.Fatal("trusted proxy client IP lost")
	}
}
