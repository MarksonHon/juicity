package server

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
)

func isNetTimeoutError(err error) bool {
	var netError net.Error
	return errors.As(err, &netError) && netError.Timeout()
}

func isGracefulStopError(ctx context.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
		return true
	}
	return ctx != nil && ctx.Err() != nil
}

func isIgnorableRelayError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, io.EOF) || isNetTimeoutError(err) || strings.HasSuffix(err.Error(), "with error code 0")
}
