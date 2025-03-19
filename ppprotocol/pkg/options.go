package ppprotocol

import "time"

// Options configures the behavior of a PPP connection
type Options struct {
	// MaxPayloadSize is the maximum size of a packet payload
	MaxPayloadSize int

	// RetransmitInterval is the base interval between retransmissions
	RetransmitInterval time.Duration

	// MaxRetries is the maximum number of retransmission attempts
	MaxRetries int

	// ConnectionTimeout is the timeout for establishing a connection
	ConnectionTimeout time.Duration

	// IdleTimeout is the timeout for considering a connection idle
	IdleTimeout time.Duration

	// KeepAliveInterval is the interval for sending keep-alive packets
	KeepAliveInterval time.Duration

	// AckDelay is the delay before sending acknowledgments (to allow for batching)
	AckDelay time.Duration

	// EnableFastRetransmit enables fast retransmission on duplicate ACKs
	EnableFastRetransmit bool

	// WindowSize controls the maximum number of unacknowledged packets
	WindowSize int

	// EnableNagling enables Nagle's algorithm for small packet coalescing
	EnableNagling bool

	// LogLevel controls the verbosity of logging
	LogLevel int
}

// DefaultOptions returns a new Options struct with default values
func DefaultOptions() *Options {
	return &Options{
		MaxPayloadSize:      1400,
		RetransmitInterval:  200 * time.Millisecond,
		MaxRetries:          10,
		ConnectionTimeout:   10 * time.Second,
		IdleTimeout:         30 * time.Second,
		KeepAliveInterval:   5 * time.Second,
		AckDelay:            20 * time.Millisecond,
		EnableFastRetransmit: true,
		WindowSize:          16,
		EnableNagling:       true,
		LogLevel:            1,
	}
}

// Merge combines the non-zero values from the provided options
func (o *Options) Merge(opts *Options) *Options {
	if opts == nil {
		return o
	}

	result := *o

	if opts.MaxPayloadSize > 0 {
		result.MaxPayloadSize = opts.MaxPayloadSize
	}

	if opts.RetransmitInterval > 0 {
		result.RetransmitInterval = opts.RetransmitInterval
	}

	if opts.MaxRetries > 0 {
		result.MaxRetries = opts.MaxRetries
	}

	if opts.ConnectionTimeout > 0 {
		result.ConnectionTimeout = opts.ConnectionTimeout
	}

	if opts.IdleTimeout > 0 {
		result.IdleTimeout = opts.IdleTimeout
	}

	if opts.KeepAliveInterval > 0 {
		result.KeepAliveInterval = opts.KeepAliveInterval
	}

	if opts.AckDelay > 0 {
		result.AckDelay = opts.AckDelay
	}

	if opts.WindowSize > 0 {
		result.WindowSize = opts.WindowSize
	}

	if opts.LogLevel >= 0 {
		result.LogLevel = opts.LogLevel
	}

	// Boolean options are just replaced
	result.EnableFastRetransmit = opts.EnableFastRetransmit
	result.EnableNagling = opts.EnableNagling

	return &result
}