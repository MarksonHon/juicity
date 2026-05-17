package server

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"

	"github.com/daeuniverse/outbound/netproxy"
	gliderLog "github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/http"
	"github.com/nadoo/glider/proxy/socks5"
)

type fakeDialer struct {
}

// Addr implements proxy.Dialer.
func (f *fakeDialer) Addr() string {
	return ""
}

// Dial implements proxy.Dialer.
func (f *fakeDialer) Dial(network string, addr string) (c net.Conn, err error) {
	return nil, fmt.Errorf("unimplemented")
}

// DialUDP implements proxy.Dialer.
func (f *fakeDialer) DialUDP(network string, addr string) (pc net.PacketConn, err error) {
	return nil, fmt.Errorf("unimplemented")
}

var defaultFakeDialer proxy.Dialer = &fakeDialer{}

type forwarder struct {
	d netproxy.Dialer
}

// Dial implements proxy.Proxy.
func (f *forwarder) Dial(network string, addr string) (c net.Conn, dialer proxy.Dialer, err error) {
	ctx, cancel := netproxy.NewDialTimeoutContext()
	defer cancel()
	conn, err := f.d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, defaultFakeDialer, err
	}
	return &netproxy.FakeNetConn{
		Conn:  conn,
		LAddr: nil,
		RAddr: nil,
	}, defaultFakeDialer, nil
}

// DialUDP implements proxy.Proxy.
func (f *forwarder) DialUDP(network string, addr string) (pc net.PacketConn, dialer proxy.UDPDialer, err error) {
	ctx, cancel := netproxy.NewDialTimeoutContext()
	defer cancel()
	conn, err := f.d.DialContext(ctx, "udp", addr)
	if err != nil {
		return nil, defaultFakeDialer, err
	}
	fc := netproxy.NewFakeNetPacketConn(conn.(netproxy.PacketConn), nil, nil)
	return fc, defaultFakeDialer, nil
}

// NextDialer implements proxy.Proxy.
func (f *forwarder) NextDialer(dstAddr string) proxy.Dialer {
	return nil
}

// Record implements proxy.Proxy.
func (f *forwarder) Record(dialer proxy.Dialer, success bool) {
}

var _ proxy.Proxy = &forwarder{}

// Mixed struct.
type Mixed struct {
	addr string

	httpServer   *http.HTTP
	socks5Server *socks5.Socks5

	mu       sync.Mutex
	listener net.Listener
}

// NewMixed returns a mixed proxy.
func NewMixed(s string, d netproxy.Dialer) (*Mixed, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("parse err: %w", err)
	}

	p := &forwarder{
		d: d,
	}
	m := &Mixed{
		addr: u.Host,
	}

	m.httpServer, err = http.NewHTTP(s, nil, p)
	if err != nil {
		return nil, err
	}

	m.socks5Server, err = socks5.NewSocks5(s, nil, p)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// ListenAndServe listens on server's addr and serves connections.
func (m *Mixed) ListenAndServe() error {
	go m.socks5Server.ListenAndServeUDP()

	l, err := net.Listen("tcp", m.addr)
	if err != nil {
		return fmt.Errorf("[mixed] failed to listen on %s: %w", m.addr, err)
	}
	m.mu.Lock()
	m.listener = l
	m.mu.Unlock()
	defer m.Close()

	gliderLog.F("[mixed] http & socks5 server listening TCP on %s", m.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("[mixed] failed to accept: %w", err)
		}

		go m.Serve(c)
	}
}

func (m *Mixed) Close() error {
	m.mu.Lock()
	l := m.listener
	m.listener = nil
	m.mu.Unlock()
	if l == nil {
		return nil
	}
	return l.Close()
}

// Serve serves connections.
func (m *Mixed) Serve(c net.Conn) {
	conn := proxy.NewConn(c)
	if head, err := conn.Peek(1); err == nil {
		if head[0] == socks5.Version {
			m.socks5Server.Serve(conn)
			return
		}
	}
	m.httpServer.Serve(conn)
}
