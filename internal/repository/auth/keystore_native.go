package auth

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/repository/keychain"
)

const (
	// nativeMarker は、新しい形の項目に付ける目印（kSecAttrComment）。古い形かどうかを見分けるためだけに使う。
	nativeMarker = "youtube-cli-native-v1"
	// nativeLabel は、新しい形の項目のラベル。
	nativeLabel = "youtube-cli"
	// migratingSuffix は、移し替えの途中で鍵を置く一時的なアカウント名の後ろに付ける。
	migratingSuffix = "#migrating"

	msgKeychainDenied = "キーチェーンの鍵の使用が拒否されました。youtube-cli をもう一度実行し、ダイアログで「常に許可」を押してください"
	msgKeychainNoUI   = "キーチェーンの確認ダイアログを出せません（SSH で入っているときなど）。この Mac の画面で一度 `youtube-cli status` を実行して許可するか、`YOUTUBE_CLI_KEYRING=file` を指定してください"
	msgMigrated       = "キーチェーンの鍵を新しい形に移しました"
)

// loadAttempts は、Load が項目を見直す回数の上限。新しい形の項目が読む前に消えたとき（ほかの youtube-cli が
// Save で作り直している途中）に見直す。消えたあとは、ないか新しい項目が見える。続けて行き違うことはまれなので、少しだけ見直す。
const loadAttempts = 3

var (
	errKeychainDenied = errors.New("キーチェーンの使用が拒否されました")
	errKeychainNoUI   = errors.New("キーチェーンの確認ダイアログを出せません")
)

// itemState は、値を読まずに見た項目の状態。
type itemState int

const (
	itemAbsent  itemState = iota
	itemNative            // 目印がある（新しい形）
	itemForeign           // 目印がない（go-keyring で作った古い形、またはほかのプログラムが置いたもの）
)

// nativeItems は、新しい形の項目の操作。サービス名は keyringService に決まっている。
type nativeItems interface {
	Inspect(account string) (itemState, error) // 値を読まない
	Get(account string) (string, error)        // ないときは errKeyringNotFound
	Add(account, secret string) error          // ACL は自分だけ、目印とラベルを付ける
	Delete(account string) error
}

// legacyItems は、go-keyring（/usr/bin/security 経由）での操作。goKeyringBackend が満たす。
type legacyItems interface {
	Get(service, user string) (string, error) // ないときは errKeyringNotFound
	Delete(service, user string) error        // ないときは errKeyringNotFound
}

var (
	_ KeyStore    = (*nativeKeyStore)(nil)
	_ legacyItems = goKeyringBackend{}
)

// nativeKeyStore は、youtube-cli だけが読める新しい形の項目に鍵を置く KeyStore。
// go-keyring で作った古い形の項目は、ロックの中の LoadLocked が Unverified で返し、Adopt で新しい形に移す（設計書 5.6）。
// Load はロックの外で呼ばれるので、何も書き換えず、古い形の項目も読まない。書き換えるのは Adopt と Save だけ（ロックの中）。
type nativeKeyStore struct {
	dir    string
	items  nativeItems
	legacy legacyItems
	guard  func() error // キーチェーンに触れる前に呼ぶ。失敗したら触れない
	notify func(msg string)
}

func newNativeKeyStore(dir string, items nativeItems, legacy legacyItems, guard func() error, notify func(msg string)) *nativeKeyStore {
	return &nativeKeyStore{dir: dir, items: items, legacy: legacy, guard: guard, notify: notify}
}

func (s *nativeKeyStore) Kind() string { return "os" }

// accounts は、本来のアカウント名と、移し替えの一時的なアカウント名。
func (s *nativeKeyStore) accounts() (account, migrating string) {
	account = keyringAccount(s.dir)
	return account, account + migratingSuffix
}

// keySource は、Load・LoadLocked が鍵を読む項目。
type keySource struct {
	account    string
	legacy     bool // 目印がないので go-keyring で読む
	unverified bool
}

// locate は、値を読まずに、鍵を読む項目を探す。なければ ok が false。
func (s *nativeKeyStore) locate() (src keySource, ok bool, err error) {
	a, t := s.accounts()
	st, err := s.items.Inspect(a)
	if err != nil {
		return keySource{}, false, nativeUnavailable(err)
	}
	switch st {
	case itemNative:
		return keySource{account: a}, true, nil
	case itemForeign:
		return keySource{account: a, legacy: true, unverified: true}, true, nil
	}
	// 本来の名前の項目がなければ、移し替えの途中で止まった一時的な項目を探す。目印のないものは使わない。
	st, err = s.items.Inspect(t)
	if err != nil {
		return keySource{}, false, nativeUnavailable(err)
	}
	if st != itemNative {
		return keySource{}, false, nil
	}
	return keySource{account: t, unverified: true}, true, nil
}

// readKey は、src の値を読んで鍵に直す。読む前に消えていたら errKeyringNotFound をそのまま返す。
func (s *nativeKeyStore) readKey(src keySource) ([]byte, error) {
	var (
		v   string
		err error
	)
	if src.legacy {
		v, err = s.legacy.Get(keyringService, src.account)
	} else {
		v, err = s.items.Get(src.account)
	}
	if errors.Is(err, errKeyringNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, nativeUnavailable(err)
	}
	return decodeKeychainKey(v)
}

// Load は、ロックの外で呼ばれる。読むのは新しい形の本来の項目だけで、何も書き換えない。
// 古い形の項目を go-keyring で読まない。Inspect で古い形と見てから /usr/bin/security が項目に届くまでの間に、
// ほかの youtube-cli が移し終えると、security が新しい形の項目に触れてダイアログが出る（「常に許可」を押すと、
// security が ACL に入って穴が開く）。古い形の項目や一時的な項目は読まずに、鍵のない Unverified の合図を返す。
func (s *nativeKeyStore) Load() (LoadedKey, bool, error) {
	if err := s.guard(); err != nil {
		return LoadedKey{}, false, err
	}
	for range loadAttempts {
		src, ok, err := s.locate()
		if err != nil || !ok {
			return LoadedKey{}, false, err
		}
		if src.unverified {
			return LoadedKey{Unverified: true}, true, nil // ロックを取って LoadLocked で読み直す合図
		}
		key, err := s.readKey(src)
		if errors.Is(err, errKeyringNotFound) {
			continue // 読む前に消えた。ほかの youtube-cli が Save で作り直している途中なので、見直す
		}
		if err != nil {
			return LoadedKey{}, false, err
		}
		return LoadedKey{Key: key}, true, nil
	}
	// 見直しても項目が消え続けた。「ない」とは言い切れないので、使えないとする。
	return LoadedKey{}, false, nativeUnavailable(errKeyringNotFound)
}

// LoadLocked は、設定ディレクトリのロックの中で呼ばれる。古い形の項目は go-keyring で、移し替えの途中なら一時的な項目を読む。
// 何も書き換えない。ほかの youtube-cli はロックの外で項目を書き換えないので、見えた項目は読むまで変わらない。
func (s *nativeKeyStore) LoadLocked() (LoadedKey, bool, error) {
	if err := s.guard(); err != nil {
		return LoadedKey{}, false, err
	}
	src, ok, err := s.locate()
	if err != nil || !ok {
		return LoadedKey{}, false, err
	}
	key, err := s.readKey(src)
	if errors.Is(err, errKeyringNotFound) {
		// youtube-cli でないプログラムが、読む前に消した。「ない」とは言い切れないので、使えないとする。
		return LoadedKey{}, false, nativeUnavailable(err)
	}
	if err != nil {
		return LoadedKey{}, false, err
	}
	return LoadedKey{Key: key, Unverified: src.unverified}, true, nil
}

// Adopt は、LoadLocked が Unverified で返した鍵を新しい形の項目に移す。設定ディレクトリのロックの中で呼ぶ。
// 途中で失敗したら、そこで止めてエラーを返す。どこで止まっても鍵はどれかの項目に残り、次の LoadLocked と Adopt が続きを行う。
func (s *nativeKeyStore) Adopt(key []byte) error {
	if err := s.guard(); err != nil {
		return err
	}
	a, t := s.accounts()
	enc := base64.StdEncoding.EncodeToString(key)
	st, err := s.items.Inspect(a)
	if err != nil {
		return nativeUnavailable(err)
	}
	switch st {
	case itemNative:
		// ロックを待つ間に、ほかの youtube-cli が移し終えている。残った一時的な項目だけを消す。知らせない。
		return s.remove(t)
	case itemForeign:
		err = s.migrateLegacy(a, t, enc)
	default:
		err = s.resumeMigrating(a, t, enc)
	}
	if err != nil {
		return err
	}
	s.notify(msgMigrated)
	return nil
}

// migrateLegacy は、古い形の項目 a を新しい形に移す。同じ名前の項目は2つ置けないので、古い項目を消す前に
// 一時的な項目 t に鍵を置き、消してから作る間に止まっても鍵を失わないようにする（設計書 5.6）。
func (s *nativeKeyStore) migrateLegacy(a, t, enc string) error {
	if err := s.remove(t); err != nil { // 前に止まったときの一時的な項目
		return err
	}
	if err := s.add(t, enc); err != nil {
		return err
	}
	if err := unlessGone(s.legacy.Delete(keyringService, a)); err != nil {
		return err
	}
	if err := s.add(a, enc); err != nil {
		return err
	}
	return unlessGone(s.items.Delete(t))
}

// resumeMigrating は、古い項目を消したあとで止まった移し替えを、一時的な項目の鍵で終える。
func (s *nativeKeyStore) resumeMigrating(a, t, enc string) error {
	if err := s.add(a, enc); err != nil {
		return err
	}
	return s.remove(t)
}

// Save は、本来の名前と一時的な名前の項目を消してから、新しい形の項目を作る。消せなければ作らない。
func (s *nativeKeyStore) Save(key []byte) error {
	if err := s.guard(); err != nil {
		return err
	}
	a, t := s.accounts()
	if err := s.remove(a); err != nil {
		return err
	}
	if err := s.remove(t); err != nil {
		return err
	}
	return s.add(a, base64.StdEncoding.EncodeToString(key))
}

func (s *nativeKeyStore) add(account, enc string) error {
	if err := s.items.Add(account, enc); err != nil {
		return nativeUnavailable(err)
	}
	return nil
}

// remove は、項目があれば消す。目印があれば自前で、なければ go-keyring で消す。
func (s *nativeKeyStore) remove(account string) error {
	st, err := s.items.Inspect(account)
	if err != nil {
		return nativeUnavailable(err)
	}
	switch st {
	case itemNative:
		return unlessGone(s.items.Delete(account))
	case itemForeign:
		return unlessGone(s.legacy.Delete(keyringService, account))
	default:
		return nil
	}
}

// unlessGone は、消したときのエラーを直す。もう消えていた（ほかの youtube-cli と行き違った）ときは成功とみなす。
func unlessGone(err error) error {
	if err == nil || errors.Is(err, errKeyringNotFound) {
		return nil
	}
	return nativeUnavailable(err)
}

// keychainErr は OSStatus を、上の決まったエラーに直す。StatusOK は nil、ないは errKeyringNotFound。
func keychainErr(st keychain.Status) error {
	switch keychain.Classify(st) {
	case keychain.ResultOK:
		return nil
	case keychain.ResultNotFound:
		return errKeyringNotFound
	case keychain.ResultDenied:
		return errKeychainDenied
	case keychain.ResultNoUI:
		return errKeychainNoUI
	default:
		return fmt.Errorf("キーチェーンの操作に失敗しました（OSStatus %d）", st)
	}
}

// nativeUnavailable は、拒否は msgKeychainDenied、ダイアログを出せないは msgKeychainNoUI、
// それ以外は osUnavailable の KindSetup のエラーにする。
func nativeUnavailable(err error) error {
	switch {
	case errors.Is(err, errKeychainDenied):
		return domain.Errorf(domain.KindSetup, msgKeychainDenied).Wrap(err)
	case errors.Is(err, errKeychainNoUI):
		return domain.Errorf(domain.KindSetup, msgKeychainNoUI).Wrap(err)
	default:
		return osUnavailable(err)
	}
}
