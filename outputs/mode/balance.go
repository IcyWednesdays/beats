package mode

import (
	"sync"
	"time"

	"github.com/elastic/libbeat/common"
	"github.com/elastic/libbeat/outputs"
)

// LoadBalancerMode balances the sending of events between multiple connections.
//
// The balancing algorithm is mostly pull-based, with multiple workers trying to pull
// some amount of work from a shared queue. Workers will try to get a new work item
// only if they have a working/active connection. Workers without active connection
// do not participate until a connection has been re-established.
// Due to the pull based nature the algorithm will load-balance events by random
// with workers having less latencies/turn-around times potentially getting more
// work items then other workers with higher latencies. Thusly the algorithm
// dynamically adapts to resource availability of server events are forwarded to.
//
// Workers not participating in the load-balancing will continuously try to reconnect
// to their configured endpoints. Once a new connection has been established,
// these workers will participate in in load-balancing again.
//
// If a connection becomes unavailable, the events are rescheduled for another
// connection to pick up. Rescheduling events is limited to a maximum number of
// send attempts. If events have not been send after maximum number of allowed
// attemps has been passed, they will be dropped.
//
// Distributing events to workers is subject to timeout. If no worker is available to
// pickup a message for sending, the message will be dropped internally.
type LoadBalancerMode struct {
	timeout   time.Duration // send/retry timeout. Every timeout is a failed send attempt
	waitRetry time.Duration // duration to wait during re-connection attempts

	// maximum number of configured send attempts. If set to 0, publisher will
	// block until event has been successfully published.
	maxAttempts int

	// waitGroup + signaling channel for handling shutdown
	wg   sync.WaitGroup
	done chan struct{}

	// channels for forwarding work items to workers.
	// The work channel is used by publisher to insert new events
	// into the load balancer. The work channel is synchronous blocking until timeout
	// for one worker available.
	// The retries channel is used to forward failed send attempts to other workers.
	// The retries channel is buffered to mitigate possible deadlocks when all
	// workers become unresponsive.
	work    chan eventsMessage
	retries chan eventsMessage
}

type eventsMessage struct {
	attemptsLeft int
	signaler     outputs.Signaler
	events       []common.MapStr
}

// NewLoadBalancerMode create a new load balancer connection mode.
func NewLoadBalancerMode(
	clients []ProtocolClient,
	maxAttempts int,
	waitRetry, timeout time.Duration,
) (*LoadBalancerMode, error) {
	m := &LoadBalancerMode{
		timeout:     timeout,
		waitRetry:   waitRetry,
		maxAttempts: maxAttempts,

		work:    make(chan eventsMessage),
		retries: make(chan eventsMessage, len(clients)*2),
		done:    make(chan struct{}),
	}
	m.start(clients)

	return m, nil
}

// Close stops all workers and closes all open connections. In flight events
// are signaled as failed.
func (m *LoadBalancerMode) Close() error {
	close(m.done)
	m.wg.Wait()
	return nil
}

// PublishEvents forwards events to some load balancing worker.
func (m *LoadBalancerMode) PublishEvents(
	signaler outputs.Signaler,
	events []common.MapStr,
) error {
	msg := eventsMessage{
		attemptsLeft: m.maxAttempts,
		signaler:     signaler,
		events:       events,
	}

	if ok := m.forwardEvent(m.work, msg); !ok {
		outputs.SignalFailed(msg.signaler)
	}
	return nil
}

func (m *LoadBalancerMode) start(clients []ProtocolClient) {
	var waitStart sync.WaitGroup
	worker := func(client ProtocolClient) {
		defer func() {
			if client.IsConnected() {
				_ = client.Close()
			}
			m.wg.Done()
		}()

		waitStart.Done()
		for {
			// reconnect loop
			for !client.IsConnected() {
				if err := client.Connect(m.timeout); err == nil {
					break
				}

				select {
				case <-m.done:
					return
				case <-time.After(m.waitRetry):
				}
			}

			// receive and process messages
			var msg eventsMessage
			select {
			case <-m.done:
				return
			case msg = <-m.retries: // receive message from other failed worker
			case msg = <-m.work: // receive message from publisher
			}
			m.onMessage(client, msg)
		}
	}

	for _, client := range clients {
		m.wg.Add(1)
		waitStart.Add(1)
		go worker(client)
	}
	waitStart.Wait()
}

func (m *LoadBalancerMode) onMessage(client ProtocolClient, msg eventsMessage) {
	published := 0
	events := msg.events
	send := 0
	for published < len(events) {
		n, err := client.PublishEvents(events[published:])
		if err != nil {
			// retry only non-confirmed subset of events in batch
			msg.events = msg.events[published:]

			// reset attempt count if subset of message has been send
			if send > 0 {
				msg.attemptsLeft = m.maxAttempts + 1
			}
			m.onFail(msg)
			return
		}
		published += n
		send++
	}
	outputs.SignalCompleted(msg.signaler)
}

func (m *LoadBalancerMode) onFail(msg eventsMessage) {
	if ok := m.forwardEvent(m.retries, msg); !ok {
		outputs.SignalFailed(msg.signaler)
	}
}

func (m *LoadBalancerMode) forwardEvent(
	ch chan eventsMessage,
	msg eventsMessage,
) bool {
	if m.maxAttempts == 0 {
		select {
		case ch <- msg:
			return true
		case <-m.done: // shutdown
			return false
		}
	} else {
		for ; msg.attemptsLeft > 0; msg.attemptsLeft-- {
			select {
			case ch <- msg:
				return true
			case <-m.done: // shutdown
				return false
			case <-time.After(m.timeout):
			}
		}
	}
	return false
}
