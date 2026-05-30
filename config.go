package main

import (
	"context"

	"github.com/sethvargo/go-envconfig"
	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/notification"
	"github.com/shimmerglass/musikat/server"
	"github.com/shimmerglass/musikat/subsonic"
	"github.com/shimmerglass/musikat/task"
)

type Config struct {
	DB       database.Config         `env:", prefix=DB_"`
	Refresh  task.Config             `env:", prefix=REFRESH_"`
	Subsonic subsonic.Config         `env:", prefix=SUBSONIC_"`
	Server   server.Config           `env:", prefix=SERVER_"`
	XMPP     notification.XMPPConfig `env:", prefix=XMPP_"`
}

func readConfig() (Config, error) {
	var cfg Config

	err := envconfig.ProcessWith(context.Background(), &envconfig.Config{
		Target:   &cfg,
		Lookuper: envconfig.PrefixLookuper("MUSIKAT_", envconfig.OsLookuper()),
	})
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}
