package navidrome

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/samber/lo"
)

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
}

func (c *Client) User(ctx context.Context, user, password string) (*User, error) {
	payload := bytes.NewReader(lo.Must(json.Marshal(authRequest{
		Username: user,
		Password: password,
	})))

	u, err := url.JoinPath(c.config.URL, "/auth/login")
	if err != nil {
		return nil, fmt.Errorf("navidrome: auth: %w", err)
	}
	res, err := c.httpClient.Post(u, "application/json", payload)
	if err != nil {
		return nil, fmt.Errorf("navidrome: auth: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("navidrome: auth: invalid status code %d", res.StatusCode)
	}

	authRes := authResponse{}
	err = json.NewDecoder(res.Body).Decode(&authRes)
	if err != nil {
		return nil, fmt.Errorf("navidrome: auth: %w", err)
	}

	return &User{
		config:     c.config,
		httpClient: c.httpClient,
		token:      authRes.Token,
	}, nil
}
