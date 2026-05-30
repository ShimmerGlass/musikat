package main

import (
	"os"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/notification"
	"github.com/shimmerglass/musikat/server"
	"github.com/shimmerglass/musikat/subsonic"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	DB       database.Config         `yaml:"db"`
	Subsonic subsonic.Config         `yaml:"subsonic"`
	Server   server.Config           `yaml:"server"`
	XMPP     notification.XMPPConfig `yaml:"xmpp"`
}

func readConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}
