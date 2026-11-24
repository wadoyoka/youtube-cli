//go:build !darwin

package auth

// newPlatformOSKeyStore は、macOS でなければ今までどおり go-keyring に鍵を置く KeyStore を返す。
func newPlatformOSKeyStore(dir string) KeyStore {
	return newOSKeyStore(dir, goKeyringBackend{})
}
