package lumberjack

// modes.go implements different sending modes for any kind of ProtocolClient.
// Currently we only support the fail-over like model used by logstash-forwarded.
//
// TODO: load-balancing mode with support for scheduling messages by computed
//       throughput in order to increase total throughput

import (
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/elastic/libbeat/common"
	"github.com/elastic/libbeat/outputs"
)

// ProtocolClient interface is a output plugin specific client implementation
// for encoding and publishing events. A ProtocolClient must be able to connection
// to it's sink and indicate connection failures in order to be reconnected byte
// the output plugin.
type ProtocolClient interface {
	// Connect establishes a connection to the clients sink.
	// The connection attempt shall report an error if no connection could been
	// established within the given time interval. A timeout value of 0 == wait
	// forever.
	Connect(timeout time.Duration) error

	// Close closes the established connection.
	Close() error

	// IsConnected indicates the clients connection state. If connection has
	// been lost while publishing events, IsConnected must return false. As long as
	// IsConnected returns false, an output plugin might try to re-establish the
	// connection by calling Connect.
	IsConnected() bool

	// PublishEvents sends events to the clients sink. On failure or timeout err
	// must be set. If connection has been lost, IsConnected must return false
	// in future calls.
	// PublishEvents is free to publish only a subset of given events, even in
	// error case. On return n indicates the number of events guaranteed to be
	// published.
	PublishEvents(events []common.MapStr) (n int, err error)
}

// ConnectionMode takes care of connecting to hosts
// and potentially doing load balancing and/or failover
type ConnectionMode interface {
	// Close will stop the modes it's publisher loop and close all it's
	// associated clients
	Close() error

	// PublishEvents will send all events (potentially asynchronous) to its
	// clients.
	PublishEvents(trans outputs.Signaler, events []common.MapStr) error
}

type singleConnectionMode struct {
	conn      ProtocolClient
	timeout   time.Duration
	waitRetry time.Duration
	closed    bool
}

// Connect to at most one host by random and swap to another host (by random) if
// active host becomes unavailable
type failOverConnectionMode struct {
	conns     []ProtocolClient
	timeout   time.Duration
	active    int
	waitRetry time.Duration
	closed    bool
}

type loadBalancerMode struct {
	timeout     time.Duration
	waitRetry   time.Duration
	maxAttempts int
	closed      bool

	wg      sync.WaitGroup
	work    chan eventsMessage
	retries chan eventsMessage
	done    chan struct{}
}

type eventsMessage struct {
	attemptsLeft int
	signaler     outputs.Signaler
	events       []common.MapStr
}

var (
	// ErrNoConnectionConfigured indicates no configured connections for publishing.
	ErrNoConnectionConfigured = errors.New("No connection configured")

	errNoActiveConnection = errors.New("No active connection")
)

func newSingleConnectionMode(
	client ProtocolClient,
	waitRetry time.Duration,
	timeout time.Duration,
) (*singleConnectionMode, error) {
	s := &singleConnectionMode{
		timeout: timeout,
		conn:    client,
	}

	_ = s.Connect() // try to connect, but ignore errors for now
	return s, nil
}

func (s *singleConnectionMode) Connect() error {
	if s.conn.IsConnected() {
		return nil
	}
	return s.conn.Connect(s.timeout)
}

func (s *singleConnectionMode) Close() error {
	s.closed = true
	return s.conn.Close()
}

func (s *singleConnectionMode) PublishEvents(
	trans outputs.Signaler,
	events []common.MapStr,
) error {
	published := 0
	for !s.closed {
		if err := s.Connect(); err != nil {
			time.Sleep(s.waitRetry)
			continue
		}

		for published < len(events) {
			n, err := s.conn.PublishEvents(events[published:])
			if err != nil {
				break
			}

			published += n
		}

		if published == len(events) {
			outputs.SignalCompleted(trans)
			return nil
		}
	}

	outputs.SignalFailed(trans)
	return nil
}

func newFailOverConnectionMode(
	clients []ProtocolClient,
	waitRetry time.Duration,
	timeout time.Duration,
) (*failOverConnectionMode, error) {
	f := &failOverConnectionMode{
		conns:     clients,
		timeout:   timeout,
		waitRetry: waitRetry,
	}

	// Try to connect preliminary, but ignore errors for now.
	// Main publisher loop is responsible to ensure an available connection.
	_ = f.Connect(-1)
	return f, nil
}

func (f *failOverConnectionMode) Close() error {
	if !f.closed {
		f.closed = true
		for _, conn := range f.conns {
			if conn.IsConnected() {
				_ = conn.Close()
			}
		}
	}
	return nil
}

func (f *failOverConnectionMode) Connect(active int) error {
	for !f.closed {
		if 0 <= active && active < len(f.conns) && f.conns[active].IsConnected() {
			return nil
		}

		var next int
		switch {
		case len(f.conns) == 0:
			return ErrNoConnectionConfigured
		case len(f.conns) == 1:
			next = 0
		case len(f.conns) == 2 && 0 <= active && active <= 1:
			next = 1 - active
		default:
			for {
				// Connect to random server to potentially spread the
				// load when large number of beats with same set of sinks
				// are started up at about the same time.
				next = rand.Int() % len(f.conns)
				if next != active {
					break
				}
			}
		}

		f.active = next
		if f.conns[next].IsConnected() {
			return nil
		}

		if err := f.conns[next].Connect(f.timeout); err != nil {
			active = next
			continue
		}

		// found active connection -> return it
		return nil
	}

	return errNoActiveConnection
}

func (f *failOverConnectionMode) PublishEvents(
	trans outputs.Signaler,
	events []common.MapStr,
) error {
	published := 0
	for !f.closed {
		if err := f.Connect(f.active); err != nil {
			continue
		}

		// loop until all events have been send in case client supports partial sends
		for published < len(events) {
			conn := f.conns[f.active]
			n, err := conn.PublishEvents(events[published:])
			if err != nil {
				// TODO(sissel): Track how frequently we timeout and reconnect. If we're
				// timing out too frequently, there's really no point in timing out since
				// basically everything is slow or down. We'll want to ratchet up the
				// timeout value slowly until things improve, then ratchet it down once
				// things seem healthy.
				time.Sleep(f.waitRetry)

				continue
			}
			published += n
		}

		if published == len(events) {
			outputs.SignalCompleted(trans)
			return nil
		}
	}

	outputs.SignalFailed(trans)
	return nil
}

func newLoadBalancerMode(
	clients []ProtocolClient,
	maxAttempts int,
	waitRetry, timeout time.Duration,
) (*loadBalancerMode, error) {
	m := &loadBalancerMode{
		timeout:     timeout,
		waitRetry:   waitRetry,
		maxAttempts: maxAttempts,
		closed:      false,

		work:    make(chan eventsMessage, len(clients)),
		retries: make(chan eventsMessage, len(clients)),
		done:    make(chan struct{}),
	}
	m.start(clients)

	return m, nil
}

func (m *loadBalancerMode) start(clients []ProtocolClient) {
	worker := func(client ProtocolClient) {
		defer func() {
			if client.IsConnected() {
				_ = client.Close()
			}
			m.wg.Done()
		}()

		// try to connect proactively
		_ = client.Connect(m.timeout)

		for {
			// reconnect loop
			for !client.IsConnected() {
				select {
				case <-m.done:
					return
				case <-time.After(m.waitRetry):
					_ = client.Connect(m.timeout)
				}
			}

			// receive and process messages
			var msg eventsMessage
			select {
			case <-m.done:
				return
			case msg = <-m.retries:
			case msg = <-m.work:
			}
			m.onMessage(client, msg)
		}
	}

	for _, client := range clients {
		m.wg.Add(1)
		go worker(client)
	}
}

func (m *loadBalancerMode) onMessage(client ProtocolClient, msg eventsMessage) {
	published := 0
	events := msg.events
	for published < len(events) {
		n, err := client.PublishEvents(events[published:])
		if err != nil {
			// retry only non-confirmed subset of events in batch
			msg.events = msg.events[published:]
			m.onFail(client, msg)
			return
		}
		published += n
	}
	outputs.SignalCompleted(msg.signaler)
}

func (m *loadBalancerMode) onFail(client ProtocolClient, msg eventsMessage) {
	msg.attemptsLeft--
	if msg.attemptsLeft <= 0 {
		outputs.SignalFailed(msg.signaler)
		return
	}
	m.retries <- msg
}

func (m *loadBalancerMode) Close() error {
	close(m.done)
	m.wg.Wait()
	return nil
}

func (m *loadBalancerMode) PublishEvents(
	signaler outputs.Signaler,
	events []common.MapStr,
) error {
	m.work <- eventsMessage{
		attemptsLeft: m.maxAttempts,
		signaler:     signaler,
		events:       events,
	}
	return nil
}
