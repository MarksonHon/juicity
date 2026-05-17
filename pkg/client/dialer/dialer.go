package dialer

import (
	"context"
	"runtime"
	"sync"

	"github.com/daeuniverse/outbound/netproxy"
	"github.com/daeuniverse/outbound/protocol/direct"
	"github.com/juicity/juicity/config"
)

var (
	protectPathMu sync.RWMutex
	protectPath   string
	protectDialMu sync.Mutex
)

func setProtectPath(path string) {
	protectPathMu.Lock()
	protectPath = path
	protectPathMu.Unlock()
}

func currentProtectPath() string {
	protectPathMu.RLock()
	path := protectPath
	protectPathMu.RUnlock()
	return path
}

type clientDialer struct {
	Dialer netproxy.Dialer
	conf   *config.Config
}

func NewClientDialer(conf *config.Config) *clientDialer {
	return &clientDialer{
		Dialer: direct.SymmetricDirect,
		conf:   conf,
	}
}

// DialContext implements netproxy.Dialer.
func (c *clientDialer) DialContext(ctx context.Context, network string, addr string) (netproxy.Conn, error) {
	if runtime.GOOS == "android" || runtime.GOOS == "linux" {
		if c.conf.ProtectPath != "" {
			// Use SoMark func
			protectDialMu.Lock()
			defer protectDialMu.Unlock()
			setProtectPath(c.conf.ProtectPath)
			defer setProtectPath("")
			magicNetwork := netproxy.MagicNetwork{
				Network: "udp",
				Mark:    114514,
			}
			return c.Dialer.DialContext(ctx, magicNetwork.Encode(), addr)
		}
	}
	return c.Dialer.DialContext(ctx, network, addr)
}

var _ netproxy.Dialer = (*clientDialer)(nil)
