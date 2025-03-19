package ppprotocol

import (
	"sync"
	"sync/atomic"
	"time"
)

// Stats tracks statistics for a PPP connection
type Stats struct {
	// Basic packet counters
	PacketsSent        atomic.Uint64
	PacketsReceived    atomic.Uint64
	BytesSent          atomic.Uint64
	BytesReceived      atomic.Uint64
	
	// Reliability metrics
	RetransmitCount    atomic.Uint64
	DuplicatesReceived atomic.Uint64
	ChecksumErrors     atomic.Uint64

	// Partial packet metrics
	PartialsSent       atomic.Uint64
	PartialsReceived   atomic.Uint64
	BytesSavedByPartial atomic.Uint64

	// Timing metrics
	startTime          time.Time
	lastActivity       time.Time
	rttSamples         []time.Duration
	rttMutex           sync.Mutex
	rttSampleCount     int
	minRTT             time.Duration
	maxRTT             time.Duration
	avgRTT             time.Duration
}

// NewStats creates a new Stats object
func NewStats() *Stats {
	now := time.Now()
	return &Stats{
		startTime:    now,
		lastActivity: now,
		rttSamples:   make([]time.Duration, 0, 100),
		minRTT:       24 * time.Hour, // Initialize to a large value
		maxRTT:       0,
		avgRTT:       0,
	}
}

// RecordPacketSent updates statistics for a sent packet
func (s *Stats) RecordPacketSent(bytes uint64, isPartial bool) {
	s.PacketsSent.Add(1)
	s.BytesSent.Add(bytes)
	s.lastActivity = time.Now()

	if isPartial {
		s.PartialsSent.Add(1)
	}
}

// RecordPacketReceived updates statistics for a received packet
func (s *Stats) RecordPacketReceived(bytes uint64, isPartial bool) {
	s.PacketsReceived.Add(1)
	s.BytesReceived.Add(bytes)
	s.lastActivity = time.Now()

	if isPartial {
		s.PartialsReceived.Add(1)
	}
}

// RecordRetransmit increments the retransmit counter
func (s *Stats) RecordRetransmit() {
	s.RetransmitCount.Add(1)
}

// RecordDuplicate increments the duplicates counter
func (s *Stats) RecordDuplicate() {
	s.DuplicatesReceived.Add(1)
}

// RecordChecksumError increments the checksum error counter
func (s *Stats) RecordChecksumError() {
	s.ChecksumErrors.Add(1)
}

// RecordBytesSaved records bytes saved by partial packet transmissions
func (s *Stats) RecordBytesSaved(bytes uint64) {
	s.BytesSavedByPartial.Add(bytes)
}

// RecordRTT adds a new RTT sample
func (s *Stats) RecordRTT(rtt time.Duration) {
	s.rttMutex.Lock()
	defer s.rttMutex.Unlock()

	// Add to samples (up to 100)
	if len(s.rttSamples) < 100 {
		s.rttSamples = append(s.rttSamples, rtt)
	} else {
		// Replace oldest sample
		s.rttSamples[s.rttSampleCount%100] = rtt
	}
	s.rttSampleCount++

	// Update min/max
	if rtt < s.minRTT {
		s.minRTT = rtt
	}
	if rtt > s.maxRTT {
		s.maxRTT = rtt
	}

	// Recalculate average
	var sum time.Duration
	for _, sample := range s.rttSamples {
		sum += sample
	}
	s.avgRTT = sum / time.Duration(len(s.rttSamples))
}

// GetRTTStats returns the current RTT statistics
func (s *Stats) GetRTTStats() (min, max, avg time.Duration) {
	s.rttMutex.Lock()
	defer s.rttMutex.Unlock()

	return s.minRTT, s.maxRTT, s.avgRTT
}

// GetUptime returns the time since the connection started
func (s *Stats) GetUptime() time.Duration {
	return time.Since(s.startTime)
}

// GetIdleTime returns the time since the last activity
func (s *Stats) GetIdleTime() time.Duration {
	return time.Since(s.lastActivity)
}

// GetSummary returns a map of key statistics
func (s *Stats) GetSummary() map[string]interface{} {
	min, max, avg := s.GetRTTStats()
	
	return map[string]interface{}{
		"uptime_seconds": s.GetUptime().Seconds(),
		"idle_seconds": s.GetIdleTime().Seconds(),
		"packets_sent": s.PacketsSent.Load(),
		"packets_received": s.PacketsReceived.Load(),
		"bytes_sent": s.BytesSent.Load(),
		"bytes_received": s.BytesReceived.Load(),
		"retransmits": s.RetransmitCount.Load(),
		"duplicates": s.DuplicatesReceived.Load(),
		"checksum_errors": s.ChecksumErrors.Load(),
		"partials_sent": s.PartialsSent.Load(),
		"partials_received": s.PartialsReceived.Load(),
		"bytes_saved": s.BytesSavedByPartial.Load(),
		"min_rtt_ms": min.Milliseconds(),
		"max_rtt_ms": max.Milliseconds(),
		"avg_rtt_ms": avg.Milliseconds(),
	}
}

// Reset resets all statistics
func (s *Stats) Reset() {
	s.PacketsSent.Store(0)
	s.PacketsReceived.Store(0)
	s.BytesSent.Store(0)
	s.BytesReceived.Store(0)
	s.RetransmitCount.Store(0)
	s.DuplicatesReceived.Store(0)
	s.ChecksumErrors.Store(0)
	s.PartialsSent.Store(0)
	s.PartialsReceived.Store(0)
	s.BytesSavedByPartial.Store(0)

	now := time.Now()
	s.startTime = now
	s.lastActivity = now

	s.rttMutex.Lock()
	defer s.rttMutex.Unlock()
	s.rttSamples = make([]time.Duration, 0, 100)
	s.rttSampleCount = 0
	s.minRTT = 24 * time.Hour
	s.maxRTT = 0
	s.avgRTT = 0
}