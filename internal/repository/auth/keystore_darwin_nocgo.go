//go:build darwin && !cgo

package auth

import "github.com/wadoyoka/youtube-cli/internal/domain"

const msgNoCgo = "この youtube-cli は cgo なしでビルドされたため、キーチェーンを使えません。Xcode Command Line Tools を入れてビルドし直すか、`YOUTUBE_CLI_KEYRING=file` を指定してください"

// newPlatformOSKeyStore は、cgo なしの macOS では、いつも使えないと返す KeyStore を返す。
// go-keyring には戻らない（youtube-cli だけが読める項目を作れないため。設計書 5.2）。
func newPlatformOSKeyStore(string) KeyStore { return unavailableKeyStore{} }

// unavailableKeyStore は、OS のキーチェーンを使えない KeyStore。何にも触れずに KindSetup を返す。
type unavailableKeyStore struct{}

func (unavailableKeyStore) Kind() string { return "os" }

func (unavailableKeyStore) err() error { return domain.Errorf(domain.KindSetup, msgNoCgo) }

func (s unavailableKeyStore) Load() (LoadedKey, bool, error)       { return LoadedKey{}, false, s.err() }
func (s unavailableKeyStore) LoadLocked() (LoadedKey, bool, error) { return s.Load() }
func (s unavailableKeyStore) Save([]byte) error                    { return s.err() }
func (s unavailableKeyStore) Adopt([]byte) error                   { return s.err() }
