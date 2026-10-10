package notification

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shimmerglass/musikat/database"
	"gosrc.io/xmpp"
	"gosrc.io/xmpp/stanza"
)

const (
	xmppMinBackoff   = 5 * time.Second
	xmppMaxBackoff   = 5 * time.Minute
	xmppPingInterval = 30 * time.Second
	xmppPingTimeout  = 15 * time.Second
	xmppSendWait     = 30 * time.Second
)

type XMPPConfig struct {
	Enabled  bool   `env:"ENABLED"`
	Host     string `env:"HOST"`
	JID      string `env:"JID"`
	Password string `env:"PASSWORD"`
}

var _ Notifier = (*XMPP)(nil)

// XMPP sends notifications over XMPP. It keeps a connection open and
// reconnects whenever it drops.
//
// gosrc.io/xmpp's StreamManager is not used: its resume path does not restart
// the receive loop, does not notice the server closing the stream, and gives
// up on stream conflicts, leaving the client silently dead. Instead a fresh
// client is created for every connection, and liveness is checked with
// XEP-0199 pings, which need a working receive loop to be answered. A failed
// send also forces a reconnection.
type XMPP struct {
	cfg    XMPPConfig
	domain string
	tls    *tls.Config // nil uses the library default

	mu     sync.Mutex
	client *xmpp.Client
	ready  chan struct{} // closed while client is connected

	kick chan struct{} // forces a reconnection
}

func NewXMPP(cfg XMPPConfig) (*XMPP, error) {
	return newXMPP(cfg, nil)
}

func newXMPP(cfg XMPPConfig, tlsConfig *tls.Config) (*XMPP, error) {
	jid, err := stanza.NewJid(cfg.JID)
	if err != nil {
		return nil, fmt.Errorf("xmpp: invalid jid: %w", err)
	}

	x := &XMPP{
		cfg:    cfg,
		domain: jid.Domain,
		tls:    tlsConfig,
		ready:  make(chan struct{}),
		kick:   make(chan struct{}, 1),
	}

	go x.run()

	return x, nil
}

func (x *XMPP) Send(ctx context.Context, to database.User, message string) error {
	if to.XMPPJID == "" {
		return fmt.Errorf("xmpp send: user has no JID")
	}

	msg := stanza.Message{Attrs: stanza.Attrs{To: to.XMPPJID}, Body: message}

	// A successful write does not mean the server got the message, e.g. when
	// it is shutting down, so a ping follows it: stanzas are processed in
	// order, so its reply acknowledges the message. On failure the connection
	// is assumed dead: reconnect and retry once, which may duplicate it.
	var failed *xmpp.Client
	for attempt := 0; ; attempt++ {
		client, err := x.connected(ctx, failed)
		if err != nil {
			return fmt.Errorf("xmpp send: %w", err)
		}

		err = client.Send(msg)
		if err == nil {
			err = x.ping(client)
		}
		if err == nil {
			return nil
		}
		if attempt > 0 {
			return fmt.Errorf("xmpp send: %w", err)
		}

		slog.Warn("xmpp send failed, reconnecting", "error", err)
		failed = client
		select {
		case x.kick <- struct{}{}:
		default:
		}
	}
}

// connected returns the current client, other than failed, waiting a bit for
// a reconnection if there is none.
func (x *XMPP) connected(ctx context.Context, failed *xmpp.Client) (*xmpp.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, xmppSendWait)
	defer cancel()

	for {
		x.mu.Lock()
		client, ready := x.client, x.ready
		x.mu.Unlock()

		if client != nil && client != failed {
			return client, nil
		}

		select {
		case <-ready:
		case <-ctx.Done():
			return nil, errors.New("not connected")
		}
	}
}

func (x *XMPP) setClient(client *xmpp.Client) {
	x.mu.Lock()
	defer x.mu.Unlock()

	x.client = client
	if client != nil {
		close(x.ready)
	} else {
		x.ready = make(chan struct{})
	}
}

func (x *XMPP) run() {
	backoff := xmppMinBackoff

	for {
		client, err := x.connect()
		if err != nil {
			slog.Error("xmpp connect failed", "error", err, "retry_in", backoff)
			time.Sleep(backoff)
			backoff = min(backoff*2, xmppMaxBackoff)
			continue
		}

		slog.Info("xmpp connected", "host", x.cfg.Host, "jid", x.cfg.JID)
		backoff = xmppMinBackoff

		// Drop reconnection requests aimed at the previous connection.
		select {
		case <-x.kick:
		default:
		}
		x.setClient(client)

		x.waitDown(client)

		x.setClient(nil)
		_ = client.Disconnect()
		time.Sleep(xmppMinBackoff)
	}
}

// connect opens a new connection.
func (x *XMPP) connect() (*xmpp.Client, error) {
	config := xmpp.Config{
		TransportConfiguration: xmpp.TransportConfiguration{
			Address:   x.cfg.Host,
			TLSConfig: x.tls,
		},
		Jid:        x.cfg.JID,
		Credential: xmpp.Password(x.cfg.Password),
		Insecure:   false,
	}

	client, err := xmpp.NewClient(&config, xmpp.NewRouter(), func(err error) {
		slog.Error("xmpp error", "error", err)
	})
	if err != nil {
		return nil, fmt.Errorf("create xmpp client: %w", err)
	}

	err = client.Connect()
	if err != nil {
		_ = client.Disconnect()
		return nil, err
	}

	return client, nil
}

// waitDown blocks until the connection stops answering pings or a send fails.
func (x *XMPP) waitDown(client *xmpp.Client) {
	ticker := time.NewTicker(xmppPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-x.kick:
			return
		case <-ticker.C:
			err := x.ping(client)
			if err != nil {
				slog.Warn("xmpp ping failed, reconnecting", "error", err)
				return
			}
		}
	}
}

func (x *XMPP) ping(client *xmpp.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), xmppPingTimeout)
	defer cancel()

	iq, err := stanza.NewIQ(stanza.Attrs{Type: stanza.IQTypeGet, To: x.domain})
	if err != nil {
		return err
	}
	iq.Payload = &xmppPing{}

	res, err := client.SendIQ(ctx, iq)
	if err != nil {
		return err
	}

	// Any reply, even an error, proves the stream is alive.
	select {
	case <-res:
		return nil
	case <-ctx.Done():
		return errors.New("ping timeout")
	}
}

// xmppPing is a XEP-0199 ping payload.
type xmppPing struct {
	XMLName xml.Name `xml:"urn:xmpp:ping ping"`
}

func (*xmppPing) Namespace() string         { return "urn:xmpp:ping" }
func (*xmppPing) GetSet() *stanza.ResultSet { return nil }
