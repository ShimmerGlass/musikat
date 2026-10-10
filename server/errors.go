package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// httpError is an error carrying the HTTP status to respond with.
type httpError struct {
	code int
	err  error
}

func (e httpError) Error() string { return e.err.Error() }
func (e httpError) Unwrap() error { return e.err }

func newHTTPError(code int, err error) error {
	return httpError{code: code, err: err}
}

func badRequest(err error) error   { return newHTTPError(http.StatusBadRequest, err) }
func unauthorized(err error) error { return newHTTPError(http.StatusUnauthorized, err) }
func forbidden(err error) error    { return newHTTPError(http.StatusForbidden, err) }
func notFound(err error) error     { return newHTTPError(http.StatusNotFound, err) }

func badRequestf(format string, args ...any) error {
	return badRequest(fmt.Errorf(format, args...))
}

// statusFromError returns the HTTP status for err, 500 for untyped errors.
func statusFromError(err error) int {
	var he httpError
	if errors.As(err, &he) {
		return he.code
	}
	return http.StatusInternalServerError
}

// localRedirect returns next if it is a local path, "/" otherwise. This
// prevents open redirects through user-supplied redirect targets.
func localRedirect(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") ||
		strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	// Browsers strip tabs and newlines from URLs, which could turn "/\t/x" into "//x".
	if strings.ContainsAny(next, "\t\r\n\\") {
		return "/"
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || !strings.HasPrefix(u.Path, "/") {
		return "/"
	}
	return next
}
