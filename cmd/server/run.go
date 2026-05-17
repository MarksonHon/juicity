package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/juicity/juicity/cmd/internal/shared"
	"github.com/juicity/juicity/config"
	"github.com/juicity/juicity/pkg/log"
	"github.com/juicity/juicity/server"

	"github.com/spf13/cobra"
)

var (
	logger = log.NewLogger(&log.Options{
		TimeFormat: time.DateTime,
	})

	runCmd = &cobra.Command{
		Use:   "run",
		Short: "To run juicity-server in the foreground.",
		RunE: func(cmd *cobra.Command, args []string) error {
			arguments := shared.GetArguments()
			conf, runLogger, err := arguments.GetConfigAndLogger()
			if err != nil {
				return err
			}
			if err = conf.ValidateForServerRun(); err != nil {
				return fmt.Errorf("invalid server config: %w", err)
			}
			logger = runLogger

			return shared.RunWithSignalCancel(logger, func(ctx context.Context) error {
				return Serve(ctx, conf)
			})
		},
	}
)

func Serve(ctx context.Context, conf *config.Config) (err error) {
	var fwmark uint64
	if conf.Fwmark != "" {
		fwmark, err = strconv.ParseUint(conf.Fwmark, 0, 32)
		if err != nil {
			return fmt.Errorf("parse fwmark: %w", err)
		}
		if fwmark > math.MaxInt || fwmark > math.MaxUint32 {
			return fmt.Errorf("fwmark is too large")
		}
	}
	s, err := server.New(&server.Options{
		Logger:                logger,
		Users:                 conf.Users,
		Certificate:           conf.Certificate,
		PrivateKey:            conf.PrivateKey,
		CongestionControl:     conf.CongestionControl,
		Fwmark:                int(fwmark),
		SendThrough:           conf.SendThrough,
		DialerLink:            conf.DialerLink,
		DisableOutboundUdp443: conf.DisableOutboundUdp443,
	})
	if err != nil {
		return err
	}
	if conf.Listen == "" {
		return fmt.Errorf(`"Listen" is required`)
	}
	logger.Info().Msg("Listen at " + conf.Listen)
	if err = s.ServeContext(ctx, conf.Listen); err != nil {
		return err
	}
	return nil
}

func init() {
	// version
	rootCmd.Version = shared.GetVersion(cgoEnabled)

	// cmds
	rootCmd.AddCommand(runCmd)

	// flags
	shared.InitArgumentsFlags(runCmd)
}
