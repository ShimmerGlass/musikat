package subsonic

import (
	"fmt"
	"net/http"
	"time"

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
		Client:     &http.Client{Timeout: 10 * time.Second},
		BaseUrl:    s.cfg.URL,
		User:       user.SubsonicUser,
		ClientName: "Musikat",
	}

	err := client.Authenticate(user.SubsonicPass)
	if err != nil {
		return nil, fmt.Errorf("subsonic auth: %w", err)
	}

	return &User{client: client}, nil
}
