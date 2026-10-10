package server

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalRedirect(t *testing.T) {
	cases := map[string]string{
		"":                       "/",
		"/":                      "/",
		"/account":               "/account",
		"/artists/x?a=b#frag":    "/artists/x?a=b#frag",
		"https://evil.example/":  "/",
		"http://evil.example":    "/",
		"//evil.example":         "/",
		"//evil.example/path":    "/",
		"/\\evil.example":        "/",
		"/\t/evil.example":       "/",
		"\\\\evil.example":       "/",
		"evil.example":           "/",
		"account":                "/",
		"javascript:alert(1)":    "/",
		"mailto:a@b.c":           "/",
		"/%2f/evil":              "/%2f/evil",
		"/ok\\evil":              "/",
		"/\nfoo":                 "/",
		"http:evil.example":      "/",
		"///evil.example":        "/",
		"/%5Cevil.example":       "/%5Cevil.example",
		"/a:b":                   "/a:b",
		"/path with space%zz":    "/",
		"https:/evil.example":    "/",
		"/\u0000":                "/",
		"/x?next=//evil.example": "/x?next=//evil.example",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			require.Equal(t, want, localRedirect(in))
		})
	}
}

func TestStatusFromError(t *testing.T) {
	require.Equal(t, http.StatusInternalServerError, statusFromError(errors.New("x")))
	require.Equal(t, http.StatusNotFound, statusFromError(notFound(errors.New("x"))))
	require.Equal(t, http.StatusForbidden, statusFromError(fmt.Errorf("wrap: %w", forbidden(errors.New("x")))))
}
