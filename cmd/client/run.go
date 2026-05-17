package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/daeuniverse/outbound/protocol"
	"github.com/daeuniverse/outbound/protocol/juicity"
	gliderLog "github.com/nadoo/glider/pkg/log"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"

	"github.com/juicity/juicity/cmd/internal/shared"
	"github.com/juicity/juicity/common"
	"github.com/juicity/juicity/config"
	"github.com/juicity/juicity/pkg/client/dialer"
	"github.com/juicity/juicity/pkg/log"
	"github.com/juicity/juicity/server"
)

const (
	quicGoDisableGSOEnv   = "QUIC_GO_DISABLE_GSO"
	quicGoDisableGSOValue = "1"
)

var (
	logger = log.NewLogger(&log.Options{
		TimeFormat: time.DateTime,
	})

	runCmd = &cobra.Command{
		Use:   "run",
		Short: "To run juicity-client in the foreground.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Some users reported that enabling GSO on the client would affect the performance of watching YouTube, so we disabled it by default.
			if _, ok := os.LookupEnv(quicGoDisableGSOEnv); !ok {
				os.Setenv(quicGoDisableGSOEnv, quicGoDisableGSOValue)
			}
			arguments := shared.GetArguments()
			conf, runLogger, err := arguments.GetConfigAndLogger()
			if err != nil {
				return err
			}
			if err = conf.ValidateForClientRun(); err != nil {
				return fmt.Errorf("invalid client config: %w", err)
			}
			logger = runLogger

			gliderLog.SetLogger(logger)

			return shared.RunWithSignalCancel(logger, func(ctx context.Context) error {
				return Serve(ctx, conf)
			})
		},
	}
)

func Serve(ctx context.Context, conf *config.Config) error {
	if conf.Sni == "" {
		conf.Sni, _, _ = net.SplitHostPort(conf.Server)
	}
	tlsConfig := &tls.Config{
		NextProtos:         []string{"h3"},
		MinVersion:         tls.VersionTLS13,
		ServerName:         conf.Sni,
		InsecureSkipVerify: conf.AllowInsecure,
	}
	if conf.PinnedCertChainSha256 != "" {
		pinnedHash, err := base64.URLEncoding.DecodeString(conf.PinnedCertChainSha256)
		if err != nil {
			pinnedHash, err = base64.StdEncoding.DecodeString(conf.PinnedCertChainSha256)
			if err != nil {
				pinnedHash, err = hex.DecodeString(conf.PinnedCertChainSha256)
				if err != nil {
					return fmt.Errorf("failed to decode PinnedCertChainSha256")
				}
			}
		}
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if !bytes.Equal(common.GenerateCertChainHash(rawCerts), pinnedHash) {
				return fmt.Errorf("pinned hash of cert chain does not match")
			}
			return nil
		}
	}
	d, err := juicity.NewDialer(dialer.NewClientDialer(conf), protocol.Header{
		ProxyAddress: conf.Server,
		Feature1:     conf.CongestionControl,
		TlsConfig:    tlsConfig,
		User:         conf.Uuid,
		Password:     conf.Password,
		IsClient:     true,
		Flags:        0,
	})
	if err != nil {
		return err
	}
	if conf.Listen == "" && len(conf.Forward) == 0 {
		return fmt.Errorf("please fill in at least one of `listen` and `forward` in the config file")
	}
	wg := pool.New().WithErrors().WithContext(ctx).WithCancelOnError()
	if conf.Listen != "" {
		s, err := server.NewMixed("mixed://"+conf.Listen, d)
		if err != nil {
			return err
		}
		wg.Go(func(ctx context.Context) error {
			return runServiceWithContext(ctx, s.ListenAndServe, s.Close)
		})
	}
	if len(conf.Forward) != 0 {
		for local, remote := range conf.Forward {
			forwarder, err := server.NewForwarder(server.ForwarderOptions{
				Logger:     logger,
				Dialer:     d,
				LocalAddr:  local,
				RemoteAddr: remote,
			})
			if err != nil {
				return err
			}
			wg.Go(func(ctx context.Context) error {
				return runServiceWithContext(ctx, forwarder.Serve, forwarder.Close)
			})
		}
	}
	return wg.Wait()
}

func runServiceWithContext(ctx context.Context, serve func() error, closeFn func() error) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- serve()
	}()

	select {
	case err := <-errCh:
		if isGracefulStopError(ctx, err) {
			return nil
		}
		return err
	case <-ctx.Done():
		_ = closeFn()
		err := <-errCh
		if isGracefulStopError(ctx, err) {
			return nil
		}
		return err
	}
}

func isGracefulStopError(ctx context.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
		return true
	}
	return ctx.Err() != nil
}

func init() {
	// version
	rootCmd.Version = shared.GetVersion(cgoEnabled)

	// cmds
	rootCmd.AddCommand(runCmd)

	shared.InitArgumentsFlags(runCmd)
}
