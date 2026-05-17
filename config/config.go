package config

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

var (
	Version = "unknown"
)

type Config struct {
	// Client
	Server                string            `json:"server"`
	Uuid                  string            `json:"uuid"`
	Password              string            `json:"password"`
	Sni                   string            `json:"sni"`
	AllowInsecure         bool              `json:"allow_insecure"`
	PinnedCertChainSha256 string            `json:"pinned_certchain_sha256"`
	ProtectPath           string            `json:"protect_path"`
	Forward               map[string]string `json:"forward"`

	// Server
	Users                 map[string]string `json:"users"`
	Certificate           string            `json:"certificate"`
	PrivateKey            string            `json:"private_key"`
	Fwmark                string            `json:"fwmark"`
	SendThrough           string            `json:"send_through"`
	DialerLink            string            `json:"dialer_link"`
	DisableOutboundUdp443 bool              `json:"disable_outbound_udp443"`

	// Common
	Listen            string `json:"listen"`
	CongestionControl string `json:"congestion_control"`
	LogLevel          string `json:"log_level"`
}

func ReadConfig(p string) (*Config, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var c Config
	if err = json.NewDecoder(f).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) ValidateForClientRun() error {
	if err := validateHostPort("server", c.Server); err != nil {
		return err
	}
	if _, err := uuid.Parse(c.Uuid); err != nil {
		return fmt.Errorf("invalid uuid: %w", err)
	}
	if c.Password == "" {
		return fmt.Errorf("password is required")
	}
	if c.Listen == "" && len(c.Forward) == 0 {
		return fmt.Errorf("at least one of listen and forward is required")
	}
	if c.Listen != "" {
		if err := validateHostPort("listen", c.Listen); err != nil {
			return err
		}
	}
	for local, remote := range c.Forward {
		if err := validateForwardLocal(local); err != nil {
			return err
		}
		if err := validateHostPort("forward target", remote); err != nil {
			return err
		}
	}
	if c.ProtectPath != "" {
		if _, err := os.Stat(c.ProtectPath); err != nil {
			return fmt.Errorf("protect_path: %w", err)
		}
	}
	return nil
}

func (c *Config) ValidateForServerRun() error {
	if err := validateHostPort("listen", c.Listen); err != nil {
		return err
	}
	if len(c.Users) == 0 {
		return fmt.Errorf("users is required")
	}
	for id, password := range c.Users {
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("invalid user uuid %q: %w", id, err)
		}
		if password == "" {
			return fmt.Errorf("password for user %q is required", id)
		}
	}
	if c.Certificate == "" {
		return fmt.Errorf("certificate is required")
	}
	if _, err := os.Stat(c.Certificate); err != nil {
		return fmt.Errorf("certificate: %w", err)
	}
	if c.PrivateKey == "" {
		return fmt.Errorf("private_key is required")
	}
	if _, err := os.Stat(c.PrivateKey); err != nil {
		return fmt.Errorf("private_key: %w", err)
	}
	if c.SendThrough != "" {
		if _, err := netip.ParseAddr(c.SendThrough); err != nil {
			return fmt.Errorf("send_through: %w", err)
		}
	}
	if c.Fwmark != "" {
		fwmark, err := strconv.ParseUint(c.Fwmark, 0, 32)
		if err != nil {
			return fmt.Errorf("fwmark: %w", err)
		}
		if fwmark > math.MaxInt || fwmark > math.MaxUint32 {
			return fmt.Errorf("fwmark is too large")
		}
	}
	return nil
}

func validateHostPort(field string, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if _, _, err := net.SplitHostPort(value); err != nil {
		return fmt.Errorf("invalid %s %q: %w", field, value, err)
	}
	return nil
}

func validateForwardLocal(local string) error {
	parts := strings.Split(local, "/")
	if len(parts) == 0 {
		return fmt.Errorf("invalid forward local address")
	}
	if err := validateHostPort("forward local address", parts[0]); err != nil {
		return err
	}
	for _, network := range parts[1:] {
		switch network {
		case "tcp", "udp":
		default:
			return fmt.Errorf("invalid forward local network %q in %q", network, local)
		}
	}
	return nil
}
