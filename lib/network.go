package lib

import (
	"context"
	"net"

	"golang.org/x/net/proxy"
)

// Adapter to turn old proxy.Dialer into ContextDialer
type contextDialerAdapter struct {
	dialer proxy.Dialer
}

type ContextDialerAdapter interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

func NewContextDialerAdapter(dialer proxy.Dialer) ContextDialerAdapter {
	return &contextDialerAdapter{dialer: dialer}
}

func (a *contextDialerAdapter) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)

	go func() {
		c, err := a.dialer.Dial(network, addr)
		if err != nil {
			errCh <- err
			return
		}
		connCh <- c
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case conn := <-connCh:
		return conn, nil
	}
}
