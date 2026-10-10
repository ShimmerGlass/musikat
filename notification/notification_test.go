package notification

import (
	"context"
	"testing"
	"time"

	"github.com/shimmerglass/musikat/database"
	"github.com/stretchr/testify/require"
	"gosrc.io/xmpp"
)

func TestNoop(t *testing.T) {
	var n Notifier = &Noop{}
	require.NoError(t, n.Send(context.Background(), database.User{XMPPJID: "a@b"}, "hi"))
	require.NoError(t, n.Send(context.Background(), database.User{}, ""))
}

func newTestXMPP() *XMPP {
	return &XMPP{
		domain: "example.org",
		ready:  make(chan struct{}),
		kick:   make(chan struct{}, 1),
	}
}

func TestXMPPSendNoJID(t *testing.T) {
	err := newTestXMPP().Send(context.Background(), database.User{}, "hi")
	require.ErrorContains(t, err, "no JID")
}

func TestXMPPSendNotConnected(t *testing.T) {
	x := newTestXMPP()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := x.Send(ctx, database.User{XMPPJID: "u@example.org"}, "hi")
	require.ErrorContains(t, err, "not connected")
	require.Less(t, time.Since(start), 5*time.Second)
}

func TestXMPPSendCanceledContext(t *testing.T) {
	x := newTestXMPP()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := x.Send(ctx, database.User{XMPPJID: "u@example.org"}, "hi")
	require.ErrorContains(t, err, "not connected")
}

func TestXMPPSetClientReady(t *testing.T) {
	x := newTestXMPP()
	select {
	case <-x.ready:
		t.Fatal("ready must be open before a client is set")
	default:
	}

	c := &xmpp.Client{}
	x.setClient(c)
	select {
	case <-x.ready:
	default:
		t.Fatal("ready must be closed while connected")
	}

	got, err := x.connected(context.Background(), nil)
	require.NoError(t, err)
	require.Same(t, c, got)

	x.setClient(nil)
	select {
	case <-x.ready:
		t.Fatal("ready must be reset after disconnect")
	default:
	}
}

func TestXMPPConnectedSkipsFailedClient(t *testing.T) {
	x := newTestXMPP()
	c1, c2 := &xmpp.Client{}, &xmpp.Client{}
	x.setClient(c1)

	// The failed client is the only one: must wait, then give up.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := x.connected(ctx, c1)
	require.ErrorContains(t, err, "not connected")

	// A reconnection while waiting unblocks the caller with the new client.
	done := make(chan *xmpp.Client, 1)
	go func() {
		c, err := x.connected(context.Background(), c1)
		if err == nil {
			done <- c
		}
	}()
	time.Sleep(20 * time.Millisecond)
	x.setClient(nil)
	x.setClient(c2)

	select {
	case c := <-done:
		require.Same(t, c2, c)
	case <-time.After(5 * time.Second):
		t.Fatal("connected did not return after reconnection")
	}
}

func TestNewXMPPInvalidJID(t *testing.T) {
	_, err := NewXMPP(XMPPConfig{JID: ""})
	require.ErrorContains(t, err, "invalid jid")
}
