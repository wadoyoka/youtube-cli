package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/domain/policy"
)

// PolicyFile は、設定ディレクトリの policy.toml。
type PolicyFile struct{ path string }

// NewPolicyFile は、dir/policy.toml を扱う PolicyFile を返す。
func NewPolicyFile(dir string) *PolicyFile {
	return &PolicyFile{path: filepath.Join(dir, "policy.toml")}
}

// Path は、policy.toml の場所を返す。
func (f *PolicyFile) Path() string { return f.path }

// CreateIfAbsent は、なければ既定の内容で 0600 で作る。作ったときだけ true を返す。
func (f *PolicyFile) CreateIfAbsent() (bool, error) {
	file, err := os.OpenFile(f.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("policy.toml を作れません: %w", err)
	}
	if _, err := file.WriteString(policy.DefaultTOML); err != nil {
		_ = file.Close()
		return false, fmt.Errorf("policy.toml を書けません: %w", err)
	}
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("policy.toml を閉じられません: %w", err)
	}
	return true, nil
}

type policyDoc struct {
	Deny []string `toml:"deny"`
}

func unreadable(cause error) error {
	return domain.Errorf(domain.KindDenied, "policy.toml を読めないため、すべてのコマンドを止めています").
		WithDetails(cause.Error()).Wrap(cause)
}

// unknownKeys は、知らないキーの名前を挙げた KindDenied を返す。ライブラリの英語の文言は出さない。
func unknownKeys(serr *toml.StrictMissingError) error {
	keys := make([]string, 0, len(serr.Errors))
	details := make([]string, 0, len(serr.Errors))
	for _, e := range serr.Errors {
		key := strings.Join(e.Key(), ".")
		row, _ := e.Position()
		keys = append(keys, key)
		details = append(details, fmt.Sprintf("%d 行目: %s", row, key))
	}
	return domain.Errorf(domain.KindDenied, "policy.toml に知らないキー（%s）があるため、すべてのコマンドを止めています。書けるキーは deny だけです", strings.Join(keys, ", ")).
		WithDetails(details...)
}

// Load は、policy.toml を読む。読めないときは、安全のため KindDenied を返す。
func (f *PolicyFile) Load() (policy.Policy, error) {
	body, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return policy.Policy{}, domain.Errorf(domain.KindSetup, "設定がありません。`youtube-cli init` を実行してください")
	}
	if err != nil {
		return policy.Policy{}, unreadable(err)
	}
	var doc policyDoc
	dec := toml.NewDecoder(bytes.NewReader(body)).DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		if serr, ok := errors.AsType[*toml.StrictMissingError](err); ok {
			return policy.Policy{}, unknownKeys(serr)
		}
		return policy.Policy{}, unreadable(err)
	}
	p := policy.Policy{Deny: doc.Deny}
	if err := p.Validate(); err != nil {
		return policy.Policy{}, err
	}
	return p, nil
}
