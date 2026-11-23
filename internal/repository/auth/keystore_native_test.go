package auth

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/repository/keychain"
)

// fakeItem は、偽物のキーチェーンの項目。native は目印があること。
type fakeItem struct {
	native bool
	value  string
}

// fakeKeychain は、新しい形の項目（nativeItems）と go-keyring（legacy()）が同じ items を見る偽物のキーチェーン。
// 本物と同じく、ACL に入っていない側から値を読んだり消したりすると、確認のダイアログの代わりに errKeychainDenied を返す。
type fakeKeychain struct {
	items map[string]fakeItem // キーはアカウント名
	fail  map[string]error    // キーは "Add:" + アカウント名のような「操作:アカウント名」
	ops   []string            // 呼ばれた操作。go-keyring の操作も同じ名前（Get・Delete）で記録する
	// hook は、操作を記録した直後に呼ぶ。ほかの youtube-cli と行き違う場面を作るのに使う。
	hook func(op string)
}

func newFakeKeychain() *fakeKeychain {
	return &fakeKeychain{items: map[string]fakeItem{}, fail: map[string]error{}}
}

func (f *fakeKeychain) record(op, account string) error {
	key := op + ":" + account
	f.ops = append(f.ops, key)
	if f.hook != nil {
		f.hook(key)
	}
	return f.fail[key]
}

func (f *fakeKeychain) Inspect(account string) (itemState, error) {
	if err := f.record("Inspect", account); err != nil {
		return itemAbsent, err
	}
	it, ok := f.items[account]
	switch {
	case !ok:
		return itemAbsent, nil
	case it.native:
		return itemNative, nil
	default:
		return itemForeign, nil
	}
}

func (f *fakeKeychain) Get(account string) (string, error) {
	if err := f.record("Get", account); err != nil {
		return "", err
	}
	it, ok := f.items[account]
	switch {
	case !ok:
		return "", errKeyringNotFound
	case !it.native: // 古い項目の ACL は youtube-cli を信頼していない
		return "", errKeychainDenied
	default:
		return it.value, nil
	}
}

func (f *fakeKeychain) Add(account, secret string) error {
	if err := f.record("Add", account); err != nil {
		return err
	}
	if _, ok := f.items[account]; ok {
		return keychainErr(keychain.StatusDuplicateItem)
	}
	f.items[account] = fakeItem{native: true, value: secret}
	return nil
}

func (f *fakeKeychain) Delete(account string) error {
	if err := f.record("Delete", account); err != nil {
		return err
	}
	it, ok := f.items[account]
	switch {
	case !ok:
		return errKeyringNotFound
	case !it.native:
		return errKeychainDenied
	default:
		delete(f.items, account)
		return nil
	}
}

func (f *fakeKeychain) legacy() legacyItems { return fakeLegacy{f: f} }

// fakeLegacy は、同じ偽物のキーチェーンを go-keyring（/usr/bin/security 経由）から見たもの。
type fakeLegacy struct{ f *fakeKeychain }

func (l fakeLegacy) Get(service, user string) (string, error) {
	if err := l.f.record("Get", user); err != nil {
		return "", err
	}
	it, ok := l.f.items[user]
	switch {
	case service != keyringService || !ok:
		return "", errKeyringNotFound
	case it.native:
		// 本物の go-keyring は目印を見ない。新しい項目の ACL は /usr/bin/security を信頼していないので、ダイアログが出る
		return "", errKeychainDenied
	default:
		return it.value, nil
	}
}

func (l fakeLegacy) Delete(service, user string) error {
	if err := l.f.record("Delete", user); err != nil {
		return err
	}
	it, ok := l.f.items[user]
	switch {
	case service != keyringService || !ok:
		return errKeyringNotFound
	case it.native: // 新しい項目の ACL は /usr/bin/security を信頼していない
		return errKeychainDenied
	default:
		delete(l.f.items, user)
		return nil
	}
}

// nativeFixture は、偽物のキーチェーンにつないだ nativeKeyStore と、本来の・一時的なアカウント名。
type nativeFixture struct {
	kc       *fakeKeychain
	store    *nativeKeyStore
	a        string // 本来のアカウント名
	tmp      string // 移し替えの一時的なアカウント名
	notified []string
}

func newNativeFixture(t *testing.T, dir string) *nativeFixture {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	fx := &nativeFixture{kc: newFakeKeychain(), a: abs, tmp: abs + "#migrating"}
	fx.store = newNativeKeyStore(dir, fx.kc, fx.kc.legacy(), func() error { return nil }, func(msg string) {
		fx.notified = append(fx.notified, msg)
	})
	return fx
}

// put は、項目を直に置く。
func (fx *nativeFixture) put(account string, native bool, value string) {
	fx.kc.items[account] = fakeItem{native: native, value: value}
}

func testNativeKey(t *testing.T) (key []byte, enc string) {
	t.Helper()
	key, err := newKey()
	if err != nil {
		t.Fatal(err)
	}
	return key, base64.StdEncoding.EncodeToString(key)
}

// mutated は、ops に Add か Delete があるか。
func mutated(ops []string) bool {
	return slices.ContainsFunc(ops, func(op string) bool {
		return strings.HasPrefix(op, "Add:") || strings.HasPrefix(op, "Delete:")
	})
}

func TestNativeKeyStore(t *testing.T) {
	t.Run("Kind は os", func(t *testing.T) {
		// Arrange
		fx := newNativeFixture(t, t.TempDir())
		// Act
		got := fx.store.Kind()
		// Assert
		if got != "os" {
			t.Fatalf("Kind = %q", got)
		}
	})

	t.Run("目印・ラベル・一時的なアカウント名の後ろは決まった値（変えると、前に作った項目を見分けられない）", func(t *testing.T) {
		// Arrange
		want := [3]string{"youtube-cli-native-v1", "youtube-cli", "#migrating"}
		// Act
		got := [3]string{nativeMarker, nativeLabel, migratingSuffix}
		// Assert
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// loads は、Load と LoadLocked。ロックの外でも中でも同じ結果になる場面で、両方を確かめる。
	loads := []struct {
		name string
		load func(s *nativeKeyStore) (LoadedKey, bool, error)
	}{
		{"Load", (*nativeKeyStore).Load},
		{"LoadLocked", (*nativeKeyStore).LoadLocked},
	}

	t.Run("新しい形の項目があれば、その鍵を Unverified=false で返す", func(t *testing.T) {
		for _, l := range loads {
			t.Run(l.name+" のとき", func(t *testing.T) {
				// Arrange
				fx := newNativeFixture(t, t.TempDir())
				key, enc := testNativeKey(t)
				fx.put(fx.a, true, enc)
				// Act
				got, found, err := l.load(fx.store)
				// Assert
				if err != nil || !found || got.Unverified || string(got.Key) != string(key) {
					t.Fatalf("found=%v unverified=%v err=%v", found, got.Unverified, err)
				}
				if mutated(fx.kc.ops) {
					t.Fatalf("書き換えた: %v", fx.kc.ops)
				}
			})
		}
	})

	t.Run("ロックの外の Load は、古い形の項目も一時的な項目も読まず、鍵のない Unverified を返す", func(t *testing.T) {
		tests := []struct {
			name    string
			account func(fx *nativeFixture) string
			native  bool
			wantOps func(fx *nativeFixture) []string
		}{
			{
				"本来の名前に目印のない項目がある",
				func(fx *nativeFixture) string { return fx.a }, false,
				func(fx *nativeFixture) []string { return []string{"Inspect:" + fx.a} },
			},
			{
				"本来の名前の項目がなく、新しい形の一時的な項目がある",
				func(fx *nativeFixture) string { return fx.tmp }, true,
				func(fx *nativeFixture) []string { return []string{"Inspect:" + fx.a, "Inspect:" + fx.tmp} },
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Arrange
				fx := newNativeFixture(t, t.TempDir())
				key, enc := testNativeKey(t)
				fx.put(tt.account(fx), tt.native, enc)
				// Act
				got, found, err := fx.store.Load()
				// Assert
				if err != nil || !found || !got.Unverified || got.Key != nil {
					t.Fatalf("key=%v found=%v unverified=%v err=%v", got.Key, found, got.Unverified, err)
				}
				if want := tt.wantOps(fx); !slices.Equal(fx.kc.ops, want) {
					t.Fatalf("ops = %v, want %v（go-keyring も一時的な項目も読まない）", fx.kc.ops, want)
				}

				// Act（ロックの中で読み直す）
				locked, found, err := fx.store.LoadLocked()
				// Assert
				if err != nil || !found || !locked.Unverified || string(locked.Key) != string(key) {
					t.Fatalf("LoadLocked: found=%v unverified=%v err=%v", found, locked.Unverified, err)
				}
			})
		}
	})

	t.Run("LoadLocked は目印のない項目を go-keyring で読み、Unverified=true で返し、何も書き換えない", func(t *testing.T) {
		// Arrange
		fx := newNativeFixture(t, t.TempDir())
		key, enc := testNativeKey(t)
		fx.put(fx.a, false, enc)
		// Act
		got, found, err := fx.store.LoadLocked()
		// Assert
		if err != nil || !found || !got.Unverified || string(got.Key) != string(key) {
			t.Fatalf("found=%v unverified=%v err=%v", found, got.Unverified, err)
		}
		if want := []string{"Inspect:" + fx.a, "Get:" + fx.a}; !slices.Equal(fx.kc.ops, want) {
			t.Fatalf("ops = %v, want %v", fx.kc.ops, want)
		}
		if it := fx.kc.items[fx.a]; it.native || it.value != enc || len(fx.kc.items) != 1 {
			t.Fatalf("items = %v", fx.kc.items)
		}
	})

	t.Run("LoadLocked は、項目がなく新しい形の一時的な項目があれば、その鍵を Unverified=true で返す", func(t *testing.T) {
		// Arrange
		fx := newNativeFixture(t, t.TempDir())
		key, enc := testNativeKey(t)
		fx.put(fx.tmp, true, enc)
		// Act
		got, found, err := fx.store.LoadLocked()
		// Assert
		if err != nil || !found || !got.Unverified || string(got.Key) != string(key) {
			t.Fatalf("found=%v unverified=%v err=%v", found, got.Unverified, err)
		}
		if mutated(fx.kc.ops) {
			t.Fatalf("書き換えた: %v", fx.kc.ops)
		}
	})

	t.Run("どちらもなければ found=false", func(t *testing.T) {
		for _, l := range loads {
			t.Run(l.name+" のとき", func(t *testing.T) {
				// Arrange
				fx := newNativeFixture(t, t.TempDir())
				// Act
				got, found, err := l.load(fx.store)
				// Assert
				if err != nil || found || got.Key != nil {
					t.Fatalf("key=%v found=%v err=%v", got.Key, found, err)
				}
			})
		}
	})

	t.Run("一時的なアカウントに目印のない項目しかなければ found=false", func(t *testing.T) {
		for _, l := range loads {
			t.Run(l.name+" のとき", func(t *testing.T) {
				// Arrange
				fx := newNativeFixture(t, t.TempDir())
				_, enc := testNativeKey(t)
				fx.put(fx.tmp, false, enc)
				// Act
				got, found, err := l.load(fx.store)
				// Assert
				if err != nil || found || got.Key != nil {
					t.Fatalf("key=%v found=%v err=%v", got.Key, found, err)
				}
				if mutated(fx.kc.ops) {
					t.Fatalf("書き換えた: %v", fx.kc.ops)
				}
			})
		}
	})

	t.Run("値が壊れていれば KindSetup の形の文言で、何も消さず作らない", func(t *testing.T) {
		broken := base64.StdEncoding.EncodeToString([]byte("short"))
		tests := []struct {
			name   string
			tmp    bool // 一時的なアカウントに置く
			native bool
		}{
			{"本来の名前の新しい形の項目", false, true},
			{"本来の名前の古い形の項目", false, false},
			{"新しい形の一時的な項目", true, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Arrange
				fx := newNativeFixture(t, t.TempDir())
				account := fx.a
				if tt.tmp {
					account = fx.tmp
				}
				fx.put(account, tt.native, broken)
				// Act
				_, found, err := fx.store.LoadLocked()
				_, _, unlockedErr := fx.store.Load()
				// Assert
				if domain.KindOf(err) != domain.KindSetup || found {
					t.Fatalf("found=%v kind=%v err=%v", found, domain.KindOf(err), err)
				}
				if !strings.Contains(err.Error(), "キーチェーンの鍵の形が正しくありません") {
					t.Fatalf("err = %v", err)
				}
				// ロックの外の Load が読むのは新しい形の本来の項目だけ。ほかは読まずに合図を返すので、形の誤りはまだ見えない。
				if readsUnlocked := !tt.tmp && tt.native; readsUnlocked != (unlockedErr != nil) {
					t.Fatalf("Load: err = %v", unlockedErr)
				}
				if unlockedErr != nil && !strings.Contains(unlockedErr.Error(), "キーチェーンの鍵の形が正しくありません") {
					t.Fatalf("Load: err = %v", unlockedErr)
				}
				if mutated(fx.kc.ops) {
					t.Fatalf("書き換えた: %v", fx.kc.ops)
				}
				if it := fx.kc.items[account]; it.native != tt.native || it.value != broken || len(fx.kc.items) != 1 {
					t.Fatalf("items = %v", fx.kc.items)
				}
			})
		}
	})

	t.Run("guard が失敗したら、キーチェーンに触れずにそのエラーを返す", func(t *testing.T) {
		guardErr := domain.Errorf(domain.KindSetup, "hardened runtime がありません")
		key, enc := testNativeKey(t)
		calls := []struct {
			name string
			call func(s *nativeKeyStore) error
		}{
			{"Load", func(s *nativeKeyStore) error { _, _, err := s.Load(); return err }},
			{"LoadLocked", func(s *nativeKeyStore) error { _, _, err := s.LoadLocked(); return err }},
			{"Save", func(s *nativeKeyStore) error { return s.Save(key) }},
			{"Adopt", func(s *nativeKeyStore) error { return s.Adopt(key) }},
		}
		for _, c := range calls {
			t.Run(c.name+" のとき", func(t *testing.T) {
				// Arrange
				kc := newFakeKeychain()
				dir := t.TempDir()
				abs, _ := filepath.Abs(dir)
				kc.items[abs] = fakeItem{native: false, value: enc}
				s := newNativeKeyStore(dir, kc, kc.legacy(), func() error { return guardErr }, func(string) {})
				// Act
				err := c.call(s)
				// Assert
				if !errors.Is(err, guardErr) {
					t.Fatalf("err = %v", err)
				}
				if len(kc.ops) != 0 {
					t.Fatalf("キーチェーンに触れた: %v", kc.ops)
				}
			})
		}
	})

	t.Run("拒否は拒否の文言、ダイアログを出せないは YOUTUBE_CLI_KEYRING=file の案内、どちらも KindSetup で、Add を呼ばない", func(t *testing.T) {
		key, enc := testNativeKey(t)
		errs := []struct {
			name  string
			err   error
			wants []string
		}{
			{"拒否", errKeychainDenied, []string{msgKeychainDenied, "常に許可"}},
			{"ダイアログを出せない", errKeychainNoUI, []string{msgKeychainNoUI, "YOUTUBE_CLI_KEYRING=file"}},
			{"そのほかの失敗", errors.New("壊れたキーチェーン"), []string{"キーチェーンが使えません", "YOUTUBE_CLI_KEYRING=file"}},
		}
		calls := []struct {
			name string
			call func(s *nativeKeyStore) error
		}{
			{"Load", func(s *nativeKeyStore) error { _, _, err := s.Load(); return err }},
			{"LoadLocked", func(s *nativeKeyStore) error { _, _, err := s.LoadLocked(); return err }},
			{"Save", func(s *nativeKeyStore) error { return s.Save(key) }},
			{"Adopt", func(s *nativeKeyStore) error { return s.Adopt(key) }},
		}
		for _, e := range errs {
			for _, c := range calls {
				t.Run(e.name+"・"+c.name+" のとき", func(t *testing.T) {
					// Arrange
					fx := newNativeFixture(t, t.TempDir())
					fx.put(fx.a, false, enc)
					fx.kc.fail["Inspect:"+fx.a] = e.err
					// Act
					err := c.call(fx.store)
					// Assert
					if domain.KindOf(err) != domain.KindSetup || !errors.Is(err, e.err) {
						t.Fatalf("kind=%v err=%v", domain.KindOf(err), err)
					}
					for _, w := range e.wants {
						if !strings.Contains(err.Error(), w) {
							t.Fatalf("%q がない: %v", w, err)
						}
					}
					if mutated(fx.kc.ops) {
						t.Fatalf("書き換えた: %v", fx.kc.ops)
					}
					if len(fx.notified) != 0 {
						t.Fatalf("知らせた: %v", fx.notified)
					}
				})
			}
		}
	})

	t.Run("古い形から移すと、古い項目が消え、新しい形に同じ鍵が入り、一時的な項目が残らず、知らせが1回", func(t *testing.T) {
		// Arrange
		fx := newNativeFixture(t, t.TempDir())
		key, enc := testNativeKey(t)
		fx.put(fx.a, false, enc)
		loaded, _, err := fx.store.LoadLocked()
		if err != nil || !loaded.Unverified {
			t.Fatalf("unverified=%v err=%v", loaded.Unverified, err)
		}
		fx.kc.ops = nil
		// Act
		err = fx.store.Adopt(loaded.Key)
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			"Inspect:" + fx.a,
			"Inspect:" + fx.tmp, "Add:" + fx.tmp,
			"Delete:" + fx.a,
			"Add:" + fx.a,
			"Delete:" + fx.tmp,
		}
		if !slices.Equal(fx.kc.ops, want) {
			t.Fatalf("ops = %v, want %v", fx.kc.ops, want)
		}
		if it, ok := fx.kc.items[fx.a]; !ok || !it.native || it.value != enc || len(fx.kc.items) != 1 {
			t.Fatalf("items = %v", fx.kc.items)
		}
		if !slices.Equal(fx.notified, []string{msgMigrated}) {
			t.Fatalf("notified = %v", fx.notified)
		}
		got, found, err := fx.store.Load()
		if err != nil || !found || got.Unverified || string(got.Key) != string(key) {
			t.Fatalf("移したあと: found=%v unverified=%v err=%v", found, got.Unverified, err)
		}
	})

	t.Run("移し替えのどの段階で止まっても、もう一度 LoadLocked と Adopt をすれば鍵を失わずに終わる", func(t *testing.T) {
		tests := []struct {
			name       string
			failOp     func(fx *nativeFixture) string
			wantNotify int // やり直しで知らせる回数
		}{
			{"一時的な項目を作るところ", func(fx *nativeFixture) string { return "Add:" + fx.tmp }, 1},
			{"古い項目を消すところ", func(fx *nativeFixture) string { return "Delete:" + fx.a }, 1},
			{"本来の名前で作るところ", func(fx *nativeFixture) string { return "Add:" + fx.a }, 1},
			{"一時的な項目を消すところ", func(fx *nativeFixture) string { return "Delete:" + fx.tmp }, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Arrange
				fx := newNativeFixture(t, t.TempDir())
				key, enc := testNativeKey(t)
				fx.put(fx.a, false, enc)
				loaded, _, err := fx.store.LoadLocked()
				if err != nil {
					t.Fatal(err)
				}
				fx.kc.fail[tt.failOp(fx)] = errors.New("ここで止まった")
				// Act
				adoptErr := fx.store.Adopt(loaded.Key)
				// Assert
				if domain.KindOf(adoptErr) != domain.KindSetup {
					t.Fatalf("kind=%v err=%v", domain.KindOf(adoptErr), adoptErr)
				}
				if len(fx.notified) != 0 {
					t.Fatalf("止まったのに知らせた: %v", fx.notified)
				}
				kept := false
				for _, it := range fx.kc.items {
					kept = kept || it.value == enc
				}
				if !kept {
					t.Fatalf("鍵を失った: %v", fx.kc.items)
				}

				// Act（やり直す）
				clear(fx.kc.fail)
				again, found, err := fx.store.LoadLocked()
				if err != nil || !found || string(again.Key) != string(key) {
					t.Fatalf("やり直しの LoadLocked: found=%v err=%v", found, err)
				}
				err = fx.store.Adopt(again.Key)
				// Assert
				if err != nil {
					t.Fatalf("やり直しの Adopt: %v", err)
				}
				if it, ok := fx.kc.items[fx.a]; !ok || !it.native || it.value != enc || len(fx.kc.items) != 1 {
					t.Fatalf("items = %v", fx.kc.items)
				}
				if len(fx.notified) != tt.wantNotify {
					t.Fatalf("notified = %v", fx.notified)
				}
			})
		}
	})

	t.Run("既に新しい形なら Adopt は何もせず、残った一時的な項目だけを消す。知らせない", func(t *testing.T) {
		tests := []struct {
			name string
			tmp  itemState // 一時的なアカウントに置く項目
		}{
			{"一時的な項目がない", itemAbsent},
			{"新しい形の一時的な項目が残っている", itemNative},
			{"目印のない一時的な項目が残っている", itemForeign},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Arrange
				fx := newNativeFixture(t, t.TempDir())
				key, enc := testNativeKey(t)
				fx.put(fx.a, true, enc)
				if tt.tmp != itemAbsent {
					fx.put(fx.tmp, tt.tmp == itemNative, enc)
				}
				// Act
				err := fx.store.Adopt(key)
				// Assert
				if err != nil {
					t.Fatal(err)
				}
				if it, ok := fx.kc.items[fx.a]; !ok || !it.native || it.value != enc || len(fx.kc.items) != 1 {
					t.Fatalf("items = %v", fx.kc.items)
				}
				if slices.ContainsFunc(fx.kc.ops, func(op string) bool { return strings.HasPrefix(op, "Add:") }) {
					t.Fatalf("作った: %v", fx.kc.ops)
				}
				if len(fx.notified) != 0 {
					t.Fatalf("知らせた: %v", fx.notified)
				}
			})
		}
	})

	t.Run("Save は新しい形の項目を置き換える", func(t *testing.T) {
		// Arrange
		fx := newNativeFixture(t, t.TempDir())
		_, oldEnc := testNativeKey(t)
		key, enc := testNativeKey(t)
		fx.put(fx.a, true, oldEnc)
		// Act
		err := fx.store.Save(key)
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if it := fx.kc.items[fx.a]; !it.native || it.value != enc || len(fx.kc.items) != 1 {
			t.Fatalf("items = %v", fx.kc.items)
		}
		if len(fx.notified) != 0 {
			t.Fatalf("知らせた: %v", fx.notified)
		}
	})

	t.Run("Save は古い形の項目を go-keyring で消してから作る", func(t *testing.T) {
		// Arrange
		fx := newNativeFixture(t, t.TempDir())
		_, oldEnc := testNativeKey(t)
		key, enc := testNativeKey(t)
		fx.put(fx.a, false, oldEnc)
		// Act
		err := fx.store.Save(key)
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"Inspect:" + fx.a, "Delete:" + fx.a, "Inspect:" + fx.tmp, "Add:" + fx.a}
		if !slices.Equal(fx.kc.ops, want) {
			t.Fatalf("ops = %v, want %v", fx.kc.ops, want)
		}
		if it := fx.kc.items[fx.a]; !it.native || it.value != enc || len(fx.kc.items) != 1 {
			t.Fatalf("items = %v", fx.kc.items)
		}
	})

	t.Run("Save は一時的な項目も消す", func(t *testing.T) {
		for _, native := range []bool{true, false} {
			// Arrange
			fx := newNativeFixture(t, t.TempDir())
			_, oldEnc := testNativeKey(t)
			key, enc := testNativeKey(t)
			fx.put(fx.tmp, native, oldEnc)
			// Act
			err := fx.store.Save(key)
			// Assert
			if err != nil {
				t.Fatalf("native=%v: %v", native, err)
			}
			if it := fx.kc.items[fx.a]; !it.native || it.value != enc || len(fx.kc.items) != 1 {
				t.Fatalf("native=%v: items = %v", native, fx.kc.items)
			}
		}
	})

	t.Run("Save で消せなければ KindSetup で、作らない", func(t *testing.T) {
		// Arrange
		fx := newNativeFixture(t, t.TempDir())
		_, oldEnc := testNativeKey(t)
		key, _ := testNativeKey(t)
		fx.put(fx.a, false, oldEnc)
		fx.kc.fail["Delete:"+fx.a] = errKeychainDenied
		// Act
		err := fx.store.Save(key)
		// Assert
		if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), msgKeychainDenied) {
			t.Fatalf("kind=%v err=%v", domain.KindOf(err), err)
		}
		if slices.ContainsFunc(fx.kc.ops, func(op string) bool { return strings.HasPrefix(op, "Add:") }) {
			t.Fatalf("作った: %v", fx.kc.ops)
		}
		if it := fx.kc.items[fx.a]; it.native || it.value != oldEnc || len(fx.kc.items) != 1 {
			t.Fatalf("items = %v", fx.kc.items)
		}
	})

	t.Run("消すときに、ほかの youtube-cli がもう消していれば、成功とみなす", func(t *testing.T) {
		// Arrange
		fx := newNativeFixture(t, t.TempDir())
		_, oldEnc := testNativeKey(t)
		key, enc := testNativeKey(t)
		fx.put(fx.a, false, oldEnc)
		fx.put(fx.tmp, true, oldEnc)
		fx.kc.hook = func(op string) {
			if op == "Delete:"+fx.a || op == "Delete:"+fx.tmp { // 消す直前に、ほかの youtube-cli が消す
				delete(fx.kc.items, strings.TrimPrefix(op, "Delete:"))
			}
		}
		// Act
		err := fx.store.Save(key)
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if it := fx.kc.items[fx.a]; !it.native || it.value != enc || len(fx.kc.items) != 1 {
			t.Fatalf("items = %v", fx.kc.items)
		}
	})

	t.Run("ロックの外の Load は、新しい形の項目が読む前に作り直されても、見直して新しい鍵を見つける", func(t *testing.T) {
		// Arrange
		fx := newNativeFixture(t, t.TempDir())
		_, oldEnc := testNativeKey(t)
		newK, newEnc := testNativeKey(t)
		fx.put(fx.a, true, oldEnc)
		vanished := false
		fx.kc.hook = func(op string) {
			switch {
			case op == "Get:"+fx.a && !vanished: // 読む直前に、ほかの youtube-cli の Save が古い鍵の項目を消す
				vanished = true
				delete(fx.kc.items, fx.a)
			case op == "Inspect:"+fx.a && vanished: // 見直す前に、新しい鍵で作り終える
				fx.kc.items[fx.a] = fakeItem{native: true, value: newEnc}
			}
		}
		// Act
		got, found, err := fx.store.Load()
		// Assert
		if err != nil || !found || got.Unverified || string(got.Key) != string(newK) {
			t.Fatalf("found=%v unverified=%v err=%v ops=%v", found, got.Unverified, err, fx.kc.ops)
		}
		if mutated(fx.kc.ops) {
			t.Fatalf("書き換えた: %v", fx.kc.ops)
		}
	})

	t.Run("設定ディレクトリのパスに空白・日本語・# があっても、一時的なアカウント名は絶対パス + #migrating で、移し替えが終わる", func(t *testing.T) {
		// Arrange
		dir := filepath.Join(t.TempDir(), "設定 #1")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			t.Fatal(err)
		}
		fx := newNativeFixture(t, dir)
		key, enc := testNativeKey(t)
		fx.put(abs, false, enc)
		// Act
		loaded, found, loadErr := fx.store.LoadLocked()
		adoptErr := fx.store.Adopt(loaded.Key)
		// Assert
		if loadErr != nil || !found || !loaded.Unverified || string(loaded.Key) != string(key) || adoptErr != nil {
			t.Fatalf("found=%v load=%v adopt=%v", found, loadErr, adoptErr)
		}
		if !slices.Contains(fx.kc.ops, "Add:"+abs+"#migrating") {
			t.Fatalf("一時的なアカウント名が違う: %v", fx.kc.ops)
		}
		if it, ok := fx.kc.items[abs]; !ok || !it.native || it.value != enc || len(fx.kc.items) != 1 {
			t.Fatalf("items = %v", fx.kc.items)
		}

		// Arrange（作り直し）
		fx.put(abs+"#migrating", true, enc)
		newK, newEnc := testNativeKey(t)
		// Act
		err = fx.store.Save(newK)
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if it, ok := fx.kc.items[abs]; !ok || !it.native || it.value != newEnc || len(fx.kc.items) != 1 {
			t.Fatalf("作り直したあと: items = %v", fx.kc.items)
		}
	})
}

func TestKeychainErr(t *testing.T) {
	tests := []struct {
		name string
		st   keychain.Status
		want error
	}{
		{"成功は nil", keychain.StatusOK, nil},
		{"項目がないは errKeyringNotFound", keychain.StatusItemNotFound, errKeyringNotFound},
		{"認証の失敗は拒否", keychain.StatusAuthFailed, errKeychainDenied},
		{"ユーザーの取り消しは拒否", keychain.StatusUserCanceled, errKeychainDenied},
		{"ダイアログを出せないは errKeychainNoUI", keychain.StatusInteractionNotAllowed, errKeychainNoUI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			st := tt.st
			// Act
			got := keychainErr(st)
			// Assert
			if !errors.Is(got, tt.want) || (tt.want == nil && got != nil) {
				t.Fatalf("keychainErr(%d) = %v, want %v", st, got, tt.want)
			}
		})
	}

	t.Run("知らない値は、決まったエラーのどれでもなく、文言に OSStatus を含む", func(t *testing.T) {
		// Arrange
		st := keychain.Status(-34018)
		// Act
		got := keychainErr(st)
		// Assert
		if got == nil {
			t.Fatal("nil")
		}
		for _, known := range []error{errKeyringNotFound, errKeychainDenied, errKeychainNoUI} {
			if errors.Is(got, known) {
				t.Fatalf("%v は %v", got, known)
			}
		}
		if !strings.Contains(got.Error(), "-34018") {
			t.Fatalf("err = %v", got)
		}
	})
}
