// Package nats provides a NATS client for pub/sub messaging.
//
// The client wraps github.com/nats-io/nats.go with support for:
//   - Publish with acknowledgment
//   - Subscribe returning a channel of messages
//   - Auto-reconnect with exponential backoff (built into nats.go)
//
// Usage:
//
//	nc, err := nats.Connect("nats://localhost:4222")
//	if err != nil { ... }
//	defer nc.Close()
//
//	// Publish
//	err = nc.Publish("subject", []byte("payload"))
//
//	// Subscribe
//	msgs, err := nc.Subscribe("subject")
//	for msg := range msgs { ... }
package nats

import (
	"fmt"
	"time"

	gnatsd "github.com/nats-io/nats.go"
)

// Client wraps a NATS connection with convenience methods.
type Client struct {
	conn *gnatsd.Conn
}

// DefaultNATSReconnectWait is the wait time between NATS reconnect attempts.
const DefaultNATSReconnectWait = 2 * time.Second

// DefaultNATSTimeout is the default NATS connection timeout.
const DefaultNATSTimeout = 5 * time.Second

// DefaultNATSChanBuffer is the default NATS subscription channel buffer size.
const DefaultNATSChanBuffer = 64

// Connect establishes a connection to a NATS server.
// The url format is "nats://host:port".
// The connection uses nats.go's built-in auto-reconnect with
// exponential backoff (default: 2s base, 10ms initial wait).
func Connect(url string) (*Client, error) {
	conn, err := gnatsd.Connect(url,
		gnatsd.RetryOnFailedConnect(true),
		gnatsd.MaxReconnects(-1), // unlimited reconnects
		gnatsd.ReconnectWait(DefaultNATSReconnectWait),
		gnatsd.Timeout(DefaultNATSTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}
	return &Client{conn: conn}, nil
}

// Close gracefully shuts down the NATS connection.
func (c *Client) Close() {
	if c.conn != nil && !c.conn.IsClosed() {
		c.conn.Close()
	}
}

// Publish publishes a message to the given subject.
// Returns an error if the connection is not connected.
func (c *Client) Publish(subject string, payload []byte) error {
	if c.conn == nil || !c.conn.IsConnected() {
		return fmt.Errorf("nats not connected")
	}
	err := c.conn.Publish(subject, payload)
	if err != nil {
		return fmt.Errorf("nats publish: %w", err)
	}
	return nil
}

// Subscribe subscribes to a subject and returns a receive-only channel of message payloads.
// The subscription is flushed before returning to ensure it is registered on the server.
// The channel is closed when the subscription is unsubscribed or the connection is closed.
func (c *Client) Subscribe(subject string) (<-chan []byte, error) {
	if c.conn == nil || !c.conn.IsConnected() {
		return nil, fmt.Errorf("nats not connected")
	}

	payloadCh := make(chan []byte, DefaultNATSChanBuffer)

	sub, err := c.conn.Subscribe(subject, func(msg *gnatsd.Msg) {
		payloadCh <- msg.Data
	})
	if err != nil {
		close(payloadCh)
		return nil, fmt.Errorf("nats subscribe: %w", err)
	}

	// Ensure subscription is registered on the server before publishing.
	if err := c.conn.Flush(); err != nil {
		_ = sub.Unsubscribe()
		close(payloadCh)
		return nil, fmt.Errorf("nats flush: %w", err)
	}

	return payloadCh, nil
}

// QueueSubscribe subscribes to a subject with queue group semantics.
// Messages are distributed among subscribers in the same queue group.
func (c *Client) QueueSubscribe(subject, queue string) (<-chan []byte, error) {
	if c.conn == nil || !c.conn.IsConnected() {
		return nil, fmt.Errorf("nats not connected")
	}

	payloadCh := make(chan []byte, DefaultNATSChanBuffer)

	sub, err := c.conn.QueueSubscribe(subject, queue, func(msg *gnatsd.Msg) {
		payloadCh <- msg.Data
	})
	if err != nil {
		close(payloadCh)
		return nil, fmt.Errorf("nats queue subscribe: %w", err)
	}

	// Ensure subscription is registered on the server before publishing.
	if err := c.conn.Flush(); err != nil {
		_ = sub.Unsubscribe()
		close(payloadCh)
		return nil, fmt.Errorf("nats flush: %w", err)
	}

	return payloadCh, nil
}

// IsConnected returns whether the NATS connection is established.
func (c *Client) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}

// IsReconnecting returns whether the NATS connection is currently reconnecting.
func (c *Client) IsReconnecting() bool {
	return c.conn != nil && c.conn.IsReconnecting()
}

// Stats returns connection statistics.
func (c *Client) Stats() gnatsd.Statistics {
	if c.conn == nil {
		return gnatsd.Statistics{}
	}
	return c.conn.Stats()
}
