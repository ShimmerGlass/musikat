package navidrome

import (
	"net/http"
	"time"
)

type Client struct {
	config     Config
	httpClient *http.Client
}

func New(config Config) *Client {
	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type User struct {
	config     Config
	httpClient *http.Client

	token string
}
