package subsonic

import (
	"fmt"
	"log"
	"net/http"

	"github.com/delucks/go-subsonic"
	"github.com/shimmerglass/musikat/database"
)

type Subsonic struct {
	cfg Config
}

type User struct {
	client *subsonic.Client
}

func New(cfg Config) *Subsonic {
	return &Subsonic{cfg: cfg}
}

func (s *Subsonic) User(user database.User) (*User, error) {
	if user.SubsonicUser == "" {
		return nil, fmt.Errorf("cannot create subsonic client: user has no subsonic user")
	}
	if user.SubsonicPass == "" {
		return nil, fmt.Errorf("cannot create subsonic client: user has no subsonic password")
	}

	client := &subsonic.Client{
		Client:     &http.Client{},
		BaseUrl:    s.cfg.URL,
		User:       user.SubsonicUser,
		ClientName: "RW",
	}

	err := client.Authenticate(user.SubsonicPass)
	if err != nil {
		log.Fatal(err)
	}

	return &User{client: client}, nil
}
