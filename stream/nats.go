package stream

import (
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/wongfei2009/harmonylite/cfg"
)

func Connect() (*nats.Conn, error) {
	// Initialize connection status to disconnected
	SetNatsConnectionStatus(false)

	opts := setupConnOptions()

	creds, err := getNatsAuthFromConfig()
	if err != nil {
		return nil, err
	}

	tls, err := getNatsTLSFromConfig()
	if err != nil {
		return nil, err
	}

	opts = append(opts, creds...)
	opts = append(opts, tls...)
	if len(cfg.Config.NATS.URLs) == 0 {
		embedded, err := startEmbeddedServer(cfg.Config.NodeName())
		if err != nil {
			return nil, err
		}

		return embedded.prepareConnection(opts...)
	}

	url := strings.Join(cfg.Config.NATS.URLs, ", ")

	var conn *nats.Conn
	for i := 0; i < cfg.Config.NATS.ConnectRetries; i++ {
		conn, err = nats.Connect(url, opts...)
		if err == nil && conn != nil && conn.Status() == nats.CONNECTED {
			SetNatsConnectionStatus(true)
			log.Info().Str("url", conn.ConnectedUrl()).Msg("NATS client connected successfully")
			return conn, nil // Successful connection
		}

		log.Warn().
			Err(err).
			Int("attempt", i+1).
			Int("attempt_limit", cfg.Config.NATS.ConnectRetries).
			Str("status", func() string {
				if conn == nil {
					return "nil connection object"
				}
				return conn.Status().String()
			}()).
			Msg("NATS connection failed")
		
		IncNatsErrorsTotal(NatsErrorTypeConnect) // Increment connect error counter
		SetNatsConnectionStatus(false)           // Ensure status is false on failed attempt

		if i < cfg.Config.NATS.ConnectRetries-1 {
			time.Sleep(time.Duration(cfg.Config.NATS.ReconnectWaitSeconds) * time.Second)
		}
	}

	// If loop finishes, connection failed
	if err == nil && (conn == nil || conn.Status() != nats.CONNECTED) {
		err = nats.ErrConnect // Ensure err is not nil if loop finishes without explicit error
	}
	return nil, err
}

func getNatsAuthFromConfig() ([]nats.Option, error) {
	opts := make([]nats.Option, 0)

	if cfg.Config.NATS.CredsUser != "" {
		opt := nats.UserInfo(cfg.Config.NATS.CredsUser, cfg.Config.NATS.CredsPassword)
		opts = append(opts, opt)
	}

	if cfg.Config.NATS.SeedFile != "" {
		opt, err := nats.NkeyOptionFromSeed(cfg.Config.NATS.SeedFile)
		if err != nil {
			return nil, err
		}

		opts = append(opts, opt)
	}

	return opts, nil
}

func getNatsTLSFromConfig() ([]nats.Option, error) {
	opts := make([]nats.Option, 0)

	if cfg.Config.NATS.CAFile != "" {
		opt := nats.RootCAs(cfg.Config.NATS.CAFile)
		opts = append(opts, opt)
	}

	if cfg.Config.NATS.CertFile != "" && cfg.Config.NATS.KeyFile != "" {
		opt := nats.ClientCert(cfg.Config.NATS.CertFile, cfg.Config.NATS.KeyFile)
		opts = append(opts, opt)
	}

	return opts, nil
}

func setupConnOptions() []nats.Option {
	return []nats.Option{
		nats.Name(cfg.Config.NodeName()),
		nats.RetryOnFailedConnect(true),
		nats.ReconnectWait(time.Duration(cfg.Config.NATS.ReconnectWaitSeconds) * time.Second),
		nats.MaxReconnects(cfg.Config.NATS.ConnectRetries),
		nats.ClosedHandler(func(nc *nats.Conn) {
			log.Error().
				Err(nc.LastError()).
				Msg("NATS client exiting")
			SetNatsConnectionStatus(false)
		}),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Error().
				Err(err).
				Msg("NATS client disconnected")
			SetNatsConnectionStatus(false)
			IncNatsErrorsTotal(NatsErrorTypeConnect) // Or a more specific error type like "disconnected"
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Info().
				Str("url", nc.ConnectedUrl()).
				Msg("NATS client reconnected")
			SetNatsConnectionStatus(true)
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			log.Error().
				Err(err).
				Str("subject", sub.Subject).
				Msg("NATS encountered an asynchronous error")
			// Consider if IncNatsErrorsTotal should be called here,
			// and what 'type' it should be. Could be too noisy if it's for all async errors.
			// For now, focusing on connection-specific errors in DisconnectErrHandler.
		}),
	}
}
