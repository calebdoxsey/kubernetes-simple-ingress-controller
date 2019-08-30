package server

type config struct {
	host    string
	port    int
	tlsPort int
}

func defaultConfig() *config {
	return &config{
		host:    "0.0.0.0",
		port:    80,
		tlsPort: 443,
	}
}

// An Option modifies the config.
type Option func(*config)

// WithHost sets the host to bind in the config.
func WithHost(host string) Option {
	return func(cfg *config) {
		cfg.host = host
	}
}

// WithPort sets the port in the config.
func WithPort(port int) Option {
	return func(cfg *config) {
		cfg.port = port
	}
}

// WithTLSPort sets the TLS port in the config.
func WithTLSPort(port int) Option {
	return func(cfg *config) {
		cfg.tlsPort = port
	}
}
