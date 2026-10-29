package auth

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// These defaults are provisional, not the final authentication policy.
type Config struct {
	PasswordMin      int
	PasswordMax      int
	ArgonMemory      uint32
	ArgonIterations  uint32
	ArgonParallelism uint8
	HashConcurrency  int
	RatePerMinute    int
	RateBurst        int
	RateMaxClients   int
	BlocklistPath    string
	TrustedProxies   []*net.IPNet
}

func DefaultConfig() Config {
	return Config{PasswordMin: 15, PasswordMax: 128, ArgonMemory: 19 * 1024,
		ArgonIterations: 2, ArgonParallelism: 1, HashConcurrency: 2,
		RatePerMinute: 5, RateBurst: 2, RateMaxClients: 10000}
}

func LoadConfig() (Config, error) {
	c := DefaultConfig()
	items := []struct {
		name   string
		target *int
	}{
		{"REGISTER_PASSWORD_MIN", &c.PasswordMin}, {"REGISTER_PASSWORD_MAX", &c.PasswordMax},
		{"REGISTER_HASH_CONCURRENCY", &c.HashConcurrency}, {"REGISTER_RATE_PER_MINUTE", &c.RatePerMinute},
		{"REGISTER_RATE_BURST", &c.RateBurst}, {"REGISTER_RATE_MAX_CLIENTS", &c.RateMaxClients},
	}
	for _, item := range items {
		if value, exists := os.LookupEnv(item.name); exists {
			n, err := strconv.Atoi(value)
			if err != nil {
				return Config{}, fmt.Errorf("invalid %s", item.name)
			}
			*item.target = n
		}
	}
	memory, iterations, parallelism := int(c.ArgonMemory), int(c.ArgonIterations), int(c.ArgonParallelism)
	for _, item := range []struct {
		name   string
		target *int
	}{
		{"REGISTER_ARGON_MEMORY_KIB", &memory}, {"REGISTER_ARGON_ITERATIONS", &iterations}, {"REGISTER_ARGON_PARALLELISM", &parallelism},
	} {
		if value, exists := os.LookupEnv(item.name); exists {
			n, err := strconv.Atoi(value)
			if err != nil {
				return Config{}, fmt.Errorf("invalid %s", item.name)
			}
			*item.target = n
		}
	}
	if memory < 19*1024 || memory > 64*1024 || iterations < 2 || iterations > 5 || parallelism < 1 || parallelism > 4 {
		return Config{}, fmt.Errorf("invalid registration Argon2 settings")
	}
	c.ArgonMemory, c.ArgonIterations, c.ArgonParallelism = uint32(memory), uint32(iterations), uint8(parallelism)
	c.BlocklistPath = os.Getenv("REGISTER_BLOCKLIST_PATH")
	if value := os.Getenv("TRUSTED_PROXY_CIDRS"); value != "" {
		for _, part := range strings.Split(value, ",") {
			_, network, err := net.ParseCIDR(strings.TrimSpace(part))
			if err != nil {
				return Config{}, fmt.Errorf("invalid TRUSTED_PROXY_CIDRS")
			}
			c.TrustedProxies = append(c.TrustedProxies, network)
		}
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if c.PasswordMin < 8 || c.PasswordMin > c.PasswordMax || c.PasswordMax < 64 || c.PasswordMax > 1024 ||
		c.HashConcurrency < 1 || c.HashConcurrency > 4 || uint64(c.ArgonMemory)*uint64(c.HashConcurrency) > 128*1024 ||
		c.ArgonMemory < 19*1024 || c.ArgonMemory > 64*1024 || c.ArgonIterations < 2 || c.ArgonIterations > 5 ||
		c.ArgonParallelism < 1 || c.ArgonParallelism > 4 || c.RatePerMinute < 1 || c.RatePerMinute > 1000 ||
		c.RateBurst < 1 || c.RateBurst > 1000 || c.RateMaxClients < 1 || c.RateMaxClients > 100000 {
		return fmt.Errorf("invalid registration settings")
	}
	return nil
}
