// Package gcloud は、setup が使う gcloud コマンドの呼び出し（usecase.CloudSetup の実装）を持つ。
package gcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/domain/gcp"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
)

// gcloudWaitDelay は、キャンセル後に孫プロセスが出力を握っていても待つ上限。
const gcloudWaitDelay = 3 * time.Second

var _ usecase.CloudSetup = (*GCloud)(nil)

// GCloud は、gcloud コマンドを許可した形でだけ呼ぶ。
type GCloud struct {
	Bin string // 空なら PATH から "gcloud" を探す
}

// NewGCloud は、PATH から gcloud を探す GCloud を返す。
func NewGCloud() *GCloud { return &GCloud{} }

func (g *GCloud) bin() string {
	if g.Bin != "" {
		return g.Bin
	}
	return "gcloud"
}

// Installed は、gcloud が見つかるかを返す。
func (g *GCloud) Installed() bool {
	_, err := exec.LookPath(g.bin())
	return err == nil
}

// allowedGCloud は、許可した呼び出しだけ true を返す（--quiet を足す前の引数で判定する）。
func allowedGCloud(args []string) bool {
	eq := func(want ...string) bool {
		if len(args) != len(want) {
			return false
		}
		for i := range want {
			if args[i] != want[i] {
				return false
			}
		}
		return true
	}
	validID := func(id string) bool { return gcp.ValidateProjectID(id) == nil }
	switch {
	case eq("auth", "list", "--filter=status:ACTIVE", "--format=json"):
		return true
	case len(args) == 4 && eq("projects", "describe", args[2], "--format=json"):
		return validID(args[2])
	case len(args) == 4 && eq("projects", "create", args[2], "--name=youtube-cli"):
		return validID(args[2])
	case len(args) == 4 && eq("services", "enable", "youtube.googleapis.com", args[3]):
		id, ok := strings.CutPrefix(args[3], "--project=")
		return ok && validID(id)
	}
	return false
}

func (g *GCloud) run(ctx context.Context, args ...string) (stdout, stderr string, err error) {
	if !allowedGCloud(args) {
		return "", "", domain.Errorf(domain.KindUnexpected, "許可していない gcloud の呼び出しです")
	}
	full := append(append([]string{}, args...), "--quiet")
	cmd := exec.CommandContext(ctx, g.bin(), full...) //nolint:gosec // 引数は allowedGCloud で確かめた固定の形だけ
	cmd.WaitDelay = gcloudWaitDelay
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if runErr := cmd.Run(); runErr != nil {
		tail := errb.String()
		if r := []rune(tail); len(r) > 2000 {
			tail = string(r[len(r)-2000:])
		}
		return out.String(), errb.String(), domain.Errorf(domain.KindUnexpected, "gcloud %s が失敗しました", strings.Join(args[:2], " ")).
			Wrap(runErr).WithDetails(tail)
	}
	return out.String(), errb.String(), nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ActiveAccount は、ログイン中のアカウントを返す。ログインしていなければ ""。
func (g *GCloud) ActiveAccount(ctx context.Context) (string, error) {
	out, _, err := g.run(ctx, "auth", "list", "--filter=status:ACTIVE", "--format=json")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		return "", nil
	}
	var accounts []struct {
		Account string `json:"account"`
	}
	if jerr := json.Unmarshal([]byte(out), &accounts); jerr != nil {
		return "", domain.Errorf(domain.KindUnexpected, "gcloud の出力を読めませんでした").Wrap(jerr)
	}
	if len(accounts) == 0 {
		return "", nil
	}
	return accounts[0].Account, nil
}

// DescribeProject は、プロジェクトの有無と状態を返す。
func (g *GCloud) DescribeProject(ctx context.Context, id string) (usecase.CloudProject, bool, error) {
	out, stderr, err := g.run(ctx, "projects", "describe", id, "--format=json")
	if err != nil {
		if containsAny(stderr, "PERMISSION_DENIED", "NOT_FOUND", "not found", "does not exist", "may not exist") {
			return usecase.CloudProject{}, false, nil
		}
		return usecase.CloudProject{}, false, err
	}
	var v struct {
		ProjectID      string `json:"projectId"`
		LifecycleState string `json:"lifecycleState"`
	}
	if jerr := json.Unmarshal([]byte(out), &v); jerr != nil {
		return usecase.CloudProject{}, false, domain.Errorf(domain.KindUnexpected, "gcloud の出力を読めませんでした").Wrap(jerr)
	}
	return usecase.CloudProject{ID: v.ProjectID, State: v.LifecycleState}, true, nil
}

// CreateProject は、プロジェクトを作る。ID が使われていれば KindConflict。
func (g *GCloud) CreateProject(ctx context.Context, id string) error {
	_, stderr, err := g.run(ctx, "projects", "create", id, "--name=youtube-cli")
	if err != nil {
		if containsAny(stderr, "already exists", "ALREADY_EXISTS", "already in use") {
			return domain.Errorf(domain.KindConflict, "プロジェクト ID %s は使われています。別の ID を指定してください", id).Wrap(err)
		}
		return err
	}
	return nil
}

// EnableYouTube は、プロジェクトで YouTube Data API を有効にする。
func (g *GCloud) EnableYouTube(ctx context.Context, id string) error {
	_, _, err := g.run(ctx, "services", "enable", "youtube.googleapis.com", "--project="+id)
	return err
}
