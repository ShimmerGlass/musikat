package notification

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/shimmerglass/musikat/database"
	"gosrc.io/xmpp"
	"gosrc.io/xmpp/stanza"
)

type XMPPConfig struct {
	Enabled  bool   `env:"ENABLED"`
	Host     string `env:"HOST"`
	JID      string `env:"JID"`
	Password string `env:"PASSWORD"`
}

var _ Notifier = (*XMPP)(nil)

type XMPP struct {
	client *xmpp.Client
}

func NewXMPP(cfg XMPPConfig) (*XMPP, error) {
	config := xmpp.Config{
		TransportConfiguration: xmpp.TransportConfiguration{
			Address: cfg.Host,
		},
		Jid:        cfg.JID,
		Credential: xmpp.Password(cfg.Password),
		Insecure:   false,
	}

	router := xmpp.NewRouter()
	client, err := xmpp.NewClient(&config, router, func(err error) {
		slog.Error(err.Error())
	})
	if err != nil {
		return nil, fmt.Errorf("create xmpp client: %w", err)
	}

	slog.Info("xmpp connecting", "host", cfg.Host, "jid", cfg.JID)
	cm := xmpp.NewStreamManager(client, nil)
	cm.PostConnect = func(c xmpp.Sender) {
		slog.Info("xmpp connected")
	}

	go func() {
		for {
			err := cm.Run()
			if err != nil {
				slog.Error(err.Error(), "component", "stream manager")
			}

			time.Sleep(5 * time.Second)
		}
	}()

	return &XMPP{client: client}, nil
}

func (x *XMPP) Send(ctx context.Context, to database.User, message string) error {
	if to.XMPPJID == "" {
		return fmt.Errorf("xmpp send: user has no JID")
	}

	err := x.client.Send(stanza.Message{Attrs: stanza.Attrs{To: to.XMPPJID}, Body: message})
	if err != nil {
		return fmt.Errorf("xmpp send: %w", err)
	}

	return nil
}
