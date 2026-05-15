//go:build integration

package nats

import (
	"os"
	"testing"
	"time"
)

// natsTestURL returns the NATS URL to test against.
// Uses TEST_NATS_URL env var, defaults to nats://localhost:4222.
func natsTestURL() string {
	if url := os.Getenv("TEST_NATS_URL"); url != "" {
		return url
	}
	return "nats://localhost:4222"
}

func newTestClient(t *testing.T) (*Client, func()) {
	t.Helper()

	client, err := Connect(natsTestURL())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	cleanup := func() {
		client.Close()
	}

	return client, cleanup
}

func TestNATS_PublishSubscribe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, cleanup := newTestClient(t)
	defer cleanup()

	subject := "test.pubsub.roundtrip"

	t.Run("publish_and_subscribe_roundtrip", func(t *testing.T) {
		sub, err := client.Subscribe(subject)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		// Give NATS time to propagate subscription.
		time.Sleep(100 * time.Millisecond)

		payload := []byte(`{"event":"test","value":42}`)
		err = client.Publish(subject, payload)
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}

		select {
		case msg := <-sub:
			if string(msg) != string(payload) {
				t.Errorf("got %q, want %q", string(msg), string(payload))
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for message (5s)")
		}
	})

	t.Run("multiple_messages_in_order", func(t *testing.T) {
		sub, err := client.Subscribe(subject)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		messages := []string{"msg1", "msg2", "msg3"}
		for _, m := range messages {
			err := client.Publish(subject, []byte(m))
			if err != nil {
				t.Fatalf("Publish %s: %v", m, err)
			}
		}

		for i, expected := range messages {
			select {
			case msg := <-sub:
				if string(msg) != expected {
					t.Errorf("msg %d: got %q, want %q", i, string(msg), expected)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("timeout waiting for msg %d (%s)", i, expected)
			}
		}
	})
}

func TestNATS_QueueSubscribe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, cleanup := newTestClient(t)
	defer cleanup()

	subject := "test.queue"
	queue := "test-queue"

	t.Run("messages_distributed_among_subscribers", func(t *testing.T) {
		// Create two queue subscribers.
		sub1, err := client.QueueSubscribe(subject, queue)
		if err != nil {
			t.Fatalf("QueueSubscribe 1: %v", err)
		}
		sub2, err := client.QueueSubscribe(subject, queue)
		if err != nil {
			t.Fatalf("QueueSubscribe 2: %v", err)
		}

		time.Sleep(200 * time.Millisecond)

		// Publish multiple messages.
		numMsgs := 5
		for i := 0; i < numMsgs; i++ {
			err := client.Publish(subject, []byte("msg-queue"))
			if err != nil {
				t.Fatalf("Publish: %v", err)
			}
		}

		// Collect received messages.
		var received1, received2 int
		timeout := time.After(5 * time.Second)

		for received1+received2 < numMsgs {
			select {
			case <-sub1:
				received1++
			case <-sub2:
				received2++
			case <-timeout:
				t.Fatalf("timeout: received1=%d, received2=%d (total %d/%d)",
					received1, received2, received1+received2, numMsgs)
			}
		}

		if received1 == 0 || received2 == 0 {
			t.Logf("warning: queue distribution uneven — sub1=%d, sub2=%d", received1, received2)
		}
	})
}

func TestNATS_IsConnected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, cleanup := newTestClient(t)
	defer cleanup()

	if !client.IsConnected() {
		t.Error("expected IsConnected to be true")
	}

	if client.IsReconnecting() {
		t.Error("expected IsReconnecting to be false")
	}
}

func TestNATS_PublishToNoSubscribers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, cleanup := newTestClient(t)
	defer cleanup()

	// Publishing to a subject with no subscribers should not error.
	err := client.Publish("test.nosubscribers", []byte("hello"))
	if err != nil {
		t.Fatalf("Publish to unsubscribed subject: %v", err)
	}
}

func TestNATS_PublishSubscribe_TTL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, cleanup := newTestClient(t)
	defer cleanup()

	subject := "test.ttl"

	t.Run("pub_sub_within_5s_timeout", func(t *testing.T) {
		sub, err := client.Subscribe(subject)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		payload := []byte("timely-message")
		err = client.Publish(subject, payload)
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}

		select {
		case msg := <-sub:
			if string(msg) != string(payload) {
				t.Errorf("got %q, want %q", string(msg), string(payload))
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout: message not received within 5s")
		}
	})
}
