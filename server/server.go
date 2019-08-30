package server

import (
	"context"
	"crypto/tls"
	"fmt"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"sync/atomic"

	"github.com/calebdoxsey/kubernetes-simple-proxy/watcher"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

// A Server serves HTTP pages.
type Server struct {
	cfg          *config
	routingTable atomic.Value
}

// New creates a new Server.
func New(options ...Option) *Server {
	cfg := defaultConfig()
	for _, o := range options {
		o(cfg)
	}
	s := &Server{
		cfg: cfg,
	}
	s.routingTable.Store(NewRoutingTable(nil))
	return s
}

// Run runs the server.
func (s *Server) Run(ctx context.Context) error {
	var eg errgroup.Group
	eg.Go(func() error {
		srv := http.Server{
			Addr:    fmt.Sprintf("%s:%d", s.cfg.host, s.cfg.tlsPort),
			Handler: s,
		}
		srv.TLSConfig = &tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				return s.routingTable.Load().(*RoutingTable).GetCertificate(hello.ServerName)
			},
		}
		log.Info().Str("addr", srv.Addr).Msg("starting secure HTTP server")
		err := srv.ListenAndServeTLS("", "")
		if err != nil {
			return fmt.Errorf("error serving tls: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		srv := http.Server{
			Addr:    fmt.Sprintf("%s:%d", s.cfg.host, s.cfg.port),
			Handler: s,
		}
		log.Info().Str("addr", srv.Addr).Msg("starting insecure HTTP server")
		err := srv.ListenAndServe()
		if err != nil {
			return fmt.Errorf("error serving non-tls: %w", err)
		}
		return nil
	})
	return eg.Wait()
}

// ServeHTTP serves an HTTP request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backendURL, err := s.routingTable.Load().(*RoutingTable).GetBackend(r.Host, r.URL.Path)
	if err != nil {
		http.Error(w, "upstream server not found", http.StatusNotFound)
		return
	}
	p := httputil.NewSingleHostReverseProxy(backendURL)
	p.ErrorLog = stdlog.New(log.Logger, "", 0)
	p.ServeHTTP(w, r)
}

// Update updates the server with new ingress rules.
func (s *Server) Update(payload *watcher.Payload) {
	s.routingTable.Store(NewRoutingTable(payload))
}
