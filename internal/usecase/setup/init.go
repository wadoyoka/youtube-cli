package setup

import "github.com/wadoyoka/youtube-cli/internal/usecase"

// Init は、設定ディレクトリと拒否リストを用意する。
type Init struct {
	Dir    usecase.ConfigDir
	Policy usecase.PolicyStore
}

// InitResult は、init の結果。
type InitResult struct {
	ConfigDir     string `json:"configDir"`
	DirCreated    bool   `json:"dirCreated"`
	PolicyPath    string `json:"policyPath"`
	PolicyCreated bool   `json:"policyCreated"`
}

// Run は、なければ作る。あるものは書き換えない。
func (u Init) Run() (InitResult, error) {
	dirCreated, err := u.Dir.Ensure()
	if err != nil {
		return InitResult{}, err
	}
	policyCreated, err := u.Policy.CreateIfAbsent()
	if err != nil {
		return InitResult{}, err
	}
	return InitResult{
		ConfigDir:     u.Dir.Path(),
		DirCreated:    dirCreated,
		PolicyPath:    u.Policy.Path(),
		PolicyCreated: policyCreated,
	}, nil
}
