package auth

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/repository/atomicfile"
)

const keyringService = "youtube-cli"

const keyLength = 32

// keyFileName は、YOUTUBE_CLI_KEYRING=file のときに鍵を置く、設定ディレクトリの中のファイルの名前。
const keyFileName = "key"

// LoadedKey は、KeyStore.Load・LoadLocked が見つけた鍵。
type LoadedKey struct {
	Key []byte
	// Unverified は、youtube-cli が作った新しい形の項目から読んだと言えない鍵（古い形の項目、移し替えの途中の一時的な項目）。
	// Load が返す Unverified は合図で、Key を持たないことがある。呼び出し側は、ロックを取って LoadLocked で読み直し、
	// 今のファイルを復号できたときだけ使い、Adopt で新しい形に移す。
	Unverified bool
}

// KeyStore は、暗号化の鍵の置き場。
type KeyStore interface {
	// Kind は "os" か "file"。不正な YOUTUBE_CLI_KEYRING のときはその値。
	Kind() string
	// Load は、確かにないときだけ (LoadedKey{}, false, nil) を返す。
	// 使えない・読めない・形が違うときは KindSetup、不正な YOUTUBE_CLI_KEYRING は KindUsage。
	// 設定ディレクトリのロックの外で呼んでよい。そのため、古い形の項目を go-keyring で読まない（読む間にほかの youtube-cli が
	// 移し終えると、/usr/bin/security が新しい形の項目に触れて、確認のダイアログが出るため）。
	// 古い形の項目や移し替えの途中の項目があれば、Key のない Unverified を返す。
	Load() (LoadedKey, bool, error)
	// LoadLocked は、設定ディレクトリのロックの中だけで呼ぶ。古い形の項目（go-keyring）も含め、どの形の鍵も読む。
	// 何も書き換えない。返す値の決まりは Load と同じ。
	LoadLocked() (LoadedKey, bool, error)
	// Save は鍵を置く。同じ名前の項目や key があれば置き換える。
	Save(key []byte) error
	// Adopt は、LoadLocked が Unverified で返した鍵を、新しい形の項目に移す。設定ディレクトリのロックの中で呼ぶ。
	Adopt(key []byte) error
}

// keyringBackend は、OS のキーチェーンとのやりとり。テストでは偽物を使う。
type keyringBackend interface {
	Get(service, user string) (string, error) // ないときは errKeyringNotFound
	Set(service, user, secret string) error
}

var errKeyringNotFound = errors.New("キーチェーンに項目がありません")

// NewKeyStore は、mode（YOUTUBE_CLI_KEYRING の値）に合う KeyStore を返す。OS のキーチェーンの KeyStore は OS ごとに違う。
func NewKeyStore(dir, mode string) KeyStore {
	switch mode {
	case "", "os":
		return newPlatformOSKeyStore(dir)
	case "file":
		return newFileKeyStore(dir)
	default:
		return invalidKeyStore{mode: mode}
	}
}

// goKeyringBackend は go-keyring を keyringBackend に合わせる。
type goKeyringBackend struct{}

func (goKeyringBackend) Get(service, user string) (string, error) {
	v, err := keyring.Get(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", errKeyringNotFound
	}
	return v, err
}

func (goKeyringBackend) Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

func (goKeyringBackend) Delete(service, user string) error {
	err := keyring.Delete(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return errKeyringNotFound
	}
	return err
}

func decodeKey(s string) ([]byte, bool) {
	key, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(key) != keyLength {
		return nil, false
	}
	return key, true
}

// decodeKeychainKey は、キーチェーンの項目の値を鍵に直す。形が違えば KindSetup。
func decodeKeychainKey(s string) ([]byte, error) {
	key, ok := decodeKey(s)
	if !ok {
		return nil, domain.Errorf(domain.KindSetup, "キーチェーンの鍵の形が正しくありません。`YOUTUBE_CLI_KEYRING=file` を指定すると、鍵を設定ディレクトリのファイルに置けます")
	}
	return key, nil
}

// keyringAccount は、キーチェーンの項目のアカウント名（設定ディレクトリの絶対パス）。
func keyringAccount(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}

// invalidKeyStore は、不正な YOUTUBE_CLI_KEYRING の値を持つ。
type invalidKeyStore struct{ mode string }

func (s invalidKeyStore) Kind() string { return s.mode }

func (s invalidKeyStore) err() error {
	return domain.Errorf(domain.KindUsage, "YOUTUBE_CLI_KEYRING の値が正しくありません（os か file を指定してください）: %s", s.mode)
}

func (s invalidKeyStore) Load() (LoadedKey, bool, error)       { return LoadedKey{}, false, s.err() }
func (s invalidKeyStore) LoadLocked() (LoadedKey, bool, error) { return s.Load() }
func (s invalidKeyStore) Save([]byte) error                    { return s.err() }
func (s invalidKeyStore) Adopt([]byte) error                   { return s.err() }

type osKeyStore struct {
	dir string
	b   keyringBackend
}

func newOSKeyStore(dir string, b keyringBackend) KeyStore {
	return &osKeyStore{dir: dir, b: b}
}

func (s *osKeyStore) Kind() string { return "os" }

func osUnavailable(err error) error {
	return domain.Errorf(domain.KindSetup, "キーチェーンが使えません。`YOUTUBE_CLI_KEYRING=file` を指定すると、鍵を設定ディレクトリのファイルに置きます（守りは弱くなります）").Wrap(err)
}

func (s *osKeyStore) Load() (LoadedKey, bool, error) {
	v, err := s.b.Get(keyringService, keyringAccount(s.dir))
	if errors.Is(err, errKeyringNotFound) {
		return LoadedKey{}, false, nil
	}
	if err != nil {
		return LoadedKey{}, false, osUnavailable(err)
	}
	key, err := decodeKeychainKey(v)
	if err != nil {
		return LoadedKey{}, false, err
	}
	return LoadedKey{Key: key}, true, nil
}

// LoadLocked は Load と同じ。Load が Unverified を返さないため。
func (s *osKeyStore) LoadLocked() (LoadedKey, bool, error) { return s.Load() }

func (s *osKeyStore) Save(key []byte) error {
	if err := s.b.Set(keyringService, keyringAccount(s.dir), base64.StdEncoding.EncodeToString(key)); err != nil {
		return osUnavailable(err)
	}
	return nil
}

// Adopt は何もしない。Load が Unverified を返さないため。
func (s *osKeyStore) Adopt([]byte) error { return nil }

type fileKeyStore struct{ dir string }

func newFileKeyStore(dir string) KeyStore { return &fileKeyStore{dir: dir} }

func (s *fileKeyStore) Kind() string { return "file" }

func (s *fileKeyStore) path() string { return filepath.Join(s.dir, keyFileName) }

func (s *fileKeyStore) unreadable(err error) error {
	e := domain.Errorf(domain.KindSetup, "鍵のファイル（%s）を読めません。消すと、暗号化したファイルは読めなくなります", s.path())
	if err != nil {
		e = e.Wrap(err)
	}
	return e
}

func (s *fileKeyStore) Load() (LoadedKey, bool, error) {
	data, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return LoadedKey{}, false, nil
	}
	if err != nil {
		return LoadedKey{}, false, s.unreadable(err)
	}
	key, ok := decodeKey(string(data))
	if !ok {
		return LoadedKey{}, false, s.unreadable(nil)
	}
	return LoadedKey{Key: key}, true, nil
}

// LoadLocked は Load と同じ。Load が Unverified を返さないため。
func (s *fileKeyStore) LoadLocked() (LoadedKey, bool, error) { return s.Load() }

// Adopt は何もしない。Load が Unverified を返さないため。
func (s *fileKeyStore) Adopt([]byte) error { return nil }

func (s *fileKeyStore) Save(key []byte) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return domain.Errorf(domain.KindSetup, "鍵のファイル（%s）を書けません", s.path()).Wrap(err)
	}
	if err := atomicfile.Write(s.path(), []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		return domain.Errorf(domain.KindSetup, "鍵のファイル（%s）を書けません", s.path()).Wrap(err)
	}
	return nil
}
