package store

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testKeyHex    = "6f3b2d1c8a4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8"
	otherKeyHex   = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	testSecretVal = "super-secret-password"
)

func TestSecretCipher_roundTrip(t *testing.T) {
	cipher, err := newSecretCipher(testKeyHex)
	require.NoError(t, err)
	require.True(t, cipher.enabled())

	sealed, err := cipher.seal(testSecretVal)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(sealed, sealedPrefix))
	assert.NotContains(t, sealed, testSecretVal)

	opened, err := cipher.open(sealed)
	require.NoError(t, err)
	assert.Equal(t, testSecretVal, opened)
}

func TestSecretCipher_noKeyStoresPlaintext(t *testing.T) {
	cipher, err := newSecretCipher("")
	require.NoError(t, err)
	require.False(t, cipher.enabled())

	sealed, err := cipher.seal(testSecretVal)
	require.NoError(t, err)
	assert.Equal(t, plainPrefix+testSecretVal, sealed)

	opened, err := cipher.open(sealed)
	require.NoError(t, err)
	assert.Equal(t, testSecretVal, opened)
}

func TestSecretCipher_sealIsRandomised(t *testing.T) {
	cipher, err := newSecretCipher(testKeyHex)
	require.NoError(t, err)

	first, err := cipher.seal(testSecretVal)
	require.NoError(t, err)

	second, err := cipher.seal(testSecretVal)
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
}

func TestSecretCipher_base64Key(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString("Zm9vYmFyYmF6cXV4Zm9vYmFyYmF6cXV4MTIzNDU2Nzg=")
	require.NoError(t, err)
	require.Len(t, raw, keyLength)

	cipher, err := newSecretCipher("Zm9vYmFyYmF6cXV4Zm9vYmFyYmF6cXV4MTIzNDU2Nzg=")
	require.NoError(t, err)
	assert.True(t, cipher.enabled())
}

func TestSecretCipher_rejectsShortKey(t *testing.T) {
	_, err := newSecretCipher("abcdef")
	require.Error(t, err)
	assert.ErrorContains(t, err, "32 bytes")
}

func TestSecretCipher_openSealedWithoutKey(t *testing.T) {
	cipher, err := newSecretCipher(testKeyHex)
	require.NoError(t, err)

	sealed, err := cipher.seal(testSecretVal)
	require.NoError(t, err)

	plaintextCipher, err := newSecretCipher("")
	require.NoError(t, err)

	_, err = plaintextCipher.open(sealed)
	require.ErrorIs(t, err, errNoKey)
}

func TestSecretCipher_openWithWrongKey(t *testing.T) {
	cipher, err := newSecretCipher(testKeyHex)
	require.NoError(t, err)

	sealed, err := cipher.seal(testSecretVal)
	require.NoError(t, err)

	other, err := newSecretCipher(otherKeyHex)
	require.NoError(t, err)

	_, err = other.open(sealed)
	require.ErrorContains(t, err, "decrypt secret")
}

func TestSecretCipher_openUnknownEncoding(t *testing.T) {
	cipher, err := newSecretCipher(testKeyHex)
	require.NoError(t, err)

	_, err = cipher.open("not-a-known-prefix")
	require.ErrorContains(t, err, "unknown encoding")
}

func TestSecretCipher_openTruncated(t *testing.T) {
	cipher, err := newSecretCipher(testKeyHex)
	require.NoError(t, err)

	_, err = cipher.open(sealedPrefix + base64.StdEncoding.EncodeToString([]byte{1, 2, 3}))
	require.ErrorContains(t, err, "truncated")
}
