package main

import (
	"flag"
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
	cfgPath := flag.String("c", "config.yaml", "Config file")
	flag.Parse()

	cfg, err := readConfig(*cfgPath)
	if err != nil {
		return err
	}

	db, err := database.New(cfg.DB)
	if err != nil {
		return err
	}

	sub := subsonic.New(cfg.Subsonic)
	mbz := musicbrainz.New()
	xmpp, err := notification.NewXMPP(cfg.XMPP)
	if err != nil {
		return err
	}

	tasks := task.New(db, mbz, sub, xmpp)
	tasks.Start()

	srv := server.New(cfg.Server, db)

	return srv.Run()
}
