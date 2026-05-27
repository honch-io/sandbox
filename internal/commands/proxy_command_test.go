package commands

import (
	"testing"

	"honch.dev/honch/internal/config"
)

func TestProxyServerAddrFormatsIPv6BindAddress(t *testing.T) {
	cfg := config.Config{
		Sandbox: config.SandboxConfig{ProxyBind: "::1"},
		Ports:   config.PortsConfig{Proxy: 18080},
	}

	addr := proxyServerAddr(cfg)
	if addr != "[::1]:18080" {
		t.Fatalf("proxyServerAddr = %q, want %q", addr, "[::1]:18080")
	}
}
