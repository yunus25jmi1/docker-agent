package remote

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/kofalt/go-memoize"

	"github.com/docker/docker-agent/pkg/desktop"
	socket "github.com/docker/docker-agent/pkg/desktop/socket"
)

var memoizer = memoize.NewMemoizer(1*time.Minute, 1*time.Minute)

// NewTransport returns an HTTP transport that uses Docker Desktop proxy if available.
func NewTransport(ctx context.Context) http.RoundTripper {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	transport := t.Clone()

	desktopRunning, err, _ := memoizer.Memoize("desktopRunning", func() (any, error) {
		return desktop.IsDockerDesktopRunning(context.Background()), nil
	})
	if err != nil {
		return transport
	}
	if running, ok := desktopRunning.(bool); ok && running && !desktop.IsWSL() {
		transport.Proxy = http.ProxyURL(&url.URL{
			Scheme: "http",
		})
		// Override the dialer to connect to the Unix socket for the proxy
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return socket.DialUnix(ctx, desktop.Paths().ProxySocket)
		}
	}

	return transport
}
