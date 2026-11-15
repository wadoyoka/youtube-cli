package usecasetest

import "github.com/wadoyoka/youtube-cli/internal/domain/policy"

// Dir は、偽の設定ディレクトリ。
type Dir struct {
	DirPath string
	Created bool
}

// Path は、パスを返す。
func (f Dir) Path() string { return f.DirPath }

// Ensure は、Created を返す。
func (f Dir) Ensure() (bool, error) { return f.Created, nil }

// Policy は、偽の拒否リストの置き場。
type Policy struct {
	FilePath string
	Created  bool
	Deny     []string
}

// Path は、パスを返す。
func (f Policy) Path() string { return f.FilePath }

// CreateIfAbsent は、Created を返す。
func (f Policy) CreateIfAbsent() (bool, error) { return f.Created, nil }

// Load は、Deny を拒否する policy.Policy を返す。
func (f Policy) Load() (policy.Policy, error) { return policy.Policy{Deny: f.Deny}, nil }
