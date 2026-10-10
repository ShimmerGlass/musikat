package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashAndCompare(t *testing.T) {
	h, err := HashPass("s3cret")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=4$"))

	ok, err := ComparePasswordAndHash("s3cret", h)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = ComparePasswordAndHash("wrong", h)
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = ComparePasswordAndHash("", h)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestHashIsSalted(t *testing.T) {
	a, err := HashPass("same")
	require.NoError(t, err)
	b, err := HashPass("same")
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestEmptyPassword(t *testing.T) {
	h, err := HashPass("")
	require.NoError(t, err)
	ok, err := ComparePasswordAndHash("", h)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestCompareInvalidHash(t *testing.T) {
	h, err := HashPass("x")
	require.NoError(t, err)
	parts := strings.Split(h, "$")

	withPart := func(i int, v string) string {
		p := append([]string(nil), parts...)
		p[i] = v
		return strings.Join(p, "$")
	}

	ok, err := ComparePasswordAndHash("x", "garbage")
	require.ErrorIs(t, err, ErrInvalidHash)
	require.False(t, ok)

	_, err = ComparePasswordAndHash("x", "")
	require.ErrorIs(t, err, ErrInvalidHash)

	_, err = ComparePasswordAndHash("x", withPart(2, "v=18"))
	require.ErrorIs(t, err, ErrIncompatibleVersion)

	for name, enc := range map[string]string{
		"bad version": withPart(2, "version"),
		"bad params":  withPart(3, "zzz"),
		"bad salt":    withPart(4, "!!!"),
		"bad hash":    withPart(5, "!!!"),
	} {
		t.Run(name, func(t *testing.T) {
			ok, err := ComparePasswordAndHash("x", enc)
			require.Error(t, err)
			require.False(t, ok)
		})
	}
}
