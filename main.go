package main

import (
	"fmt"
	"os"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/musicbrainz"
	"github.com/shimmerglass/musikat/notification"
	"github.com/shimmerglass/musikat/server"
	"github.com/shimmerglass/musikat/subsonic"
	"github.com/shimmerglass/musikat/task"
)

func main() {
	err := run()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := readConfig()
	if err != nil {
		return err
	}

	db, err := database.New(cfg.DB)
	if err != nil {
		return err
	}

	sub := subsonic.New(cfg.Subsonic)
	mbz := musicbrainz.New()

	var notif notification.Notifier
	if cfg.XMPP.Enabled {
		notif, err = notification.NewXMPP(cfg.XMPP)
		if err != nil {
			return err
		}
	} else {
		notif = &notification.Noop{}
	}

	tasks := task.New(cfg.Refresh, db, mbz, sub, notif)
	tasks.Start()

	srv := server.New(cfg.Server, db, tasks)

	return srv.Run()
}
