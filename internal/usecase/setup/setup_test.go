package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/domain"
	"github.com/wadoyoka/youtube-cli/internal/domain/gcp"
	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/account"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

// fakeCloud は、呼ばれた順を覚える偽の CloudSetup。
type fakeCloud struct {
	missing      bool
	account      string
	projects     map[string]usecase.CloudProject
	taken        map[string]bool // CreateProject が KindConflict を返す ID
	calls        []string
	enableFails  int // EnableYouTube が先に失敗する回数
	enableCalls  int
	enableErrMsg string
	onDescribe   func(ctx context.Context) error // nil でなければ DescribeProject で呼び、その誤りを返す
}

func (c *fakeCloud) Installed() bool {
	c.calls = append(c.calls, "installed")
	return !c.missing
}

func (c *fakeCloud) ActiveAccount(context.Context) (string, error) {
	c.calls = append(c.calls, "account")
	return c.account, nil
}

func (c *fakeCloud) DescribeProject(ctx context.Context, id string) (usecase.CloudProject, bool, error) {
	c.calls = append(c.calls, "describe:"+id)
	if c.onDescribe != nil {
		if err := c.onDescribe(ctx); err != nil {
			return usecase.CloudProject{}, false, err
		}
	}
	p, ok := c.projects[id]
	return p, ok, nil
}

func (c *fakeCloud) CreateProject(_ context.Context, id string) error {
	c.calls = append(c.calls, "create:"+id)
	if c.taken[id] {
		return domain.Errorf(domain.KindConflict, "ID %s は使われています", id)
	}
	c.projects[id] = usecase.CloudProject{ID: id, State: "ACTIVE"}
	return nil
}

func (c *fakeCloud) EnableYouTube(_ context.Context, id string) error {
	c.calls = append(c.calls, "enable:"+id)
	c.enableCalls++
	if c.enableCalls <= c.enableFails {
		return errors.New(c.enableErrMsg)
	}
	return nil
}

type askAnswer struct {
	val string
	err error
}

// fakePrompter は、答えを順に返し、尋ねたことと案内を記録する。
type fakePrompter struct {
	answers  []askAnswer
	confirms []bool
	asked    []string
	defs     []string
	confirmQ []string
	said     []string
}

func (p *fakePrompter) Ask(q, def string) (string, error) {
	p.asked = append(p.asked, q)
	p.defs = append(p.defs, def)
	if len(p.answers) == 0 {
		return "", io.EOF
	}
	a := p.answers[0]
	p.answers = p.answers[1:]
	if a.err != nil {
		return "", a.err
	}
	if a.val == "" {
		return def, nil
	}
	return a.val, nil
}

func (p *fakePrompter) Confirm(q string) (bool, error) {
	p.confirmQ = append(p.confirmQ, q)
	if len(p.confirms) == 0 {
		return false, nil
	}
	c := p.confirms[0]
	p.confirms = p.confirms[1:]
	return c, nil
}

func (p *fakePrompter) Say(format string, args ...any) {
	p.said = append(p.said, fmt.Sprintf(format, args...))
}

func (p *fakePrompter) saidContains(s string) bool {
	return slices.ContainsFunc(p.said, func(l string) bool { return strings.Contains(l, s) })
}

const defaultSetupID = "youtube-cli-abc123"

var testChannel = domain.Channel{ID: "UCtest", Title: "テストチャンネル", UploadsPlaylistID: "UUtest"}

type setupFixture struct {
	auth   *usecasetest.Auth
	cloud  *fakeCloud
	ask    *fakePrompter
	opened []string
	waits  []time.Duration
	policy usecasetest.Policy
	u      Setup
}

func newSetupFixture(t *testing.T) *setupFixture {
	t.Helper()
	f := &setupFixture{
		auth: &usecasetest.Auth{
			KeyStore:       "os",
			SecretProjects: map[string]string{},
			SecretErrs:     map[string]error{},
		},
		cloud: &fakeCloud{
			account:  "owner@example.com",
			projects: map[string]usecase.CloudProject{},
			taken:    map[string]bool{},
		},
		ask:    &fakePrompter{},
		policy: usecasetest.Policy{FilePath: "/cfg/policy.toml"},
	}
	remote := usecase.Remote{
		Open:   usecasetest.Opener(&usecasetest.YouTube{Channel: testChannel}),
		State:  &usecasetest.State{Saved: testChannel, Found: true},
		Locker: &usecasetest.Locker{},
	}
	f.u = Setup{
		Init:         Init{Dir: usecasetest.Dir{DirPath: "/cfg"}, Policy: f.policy},
		Policy:       f.policy,
		Auth:         f.auth,
		Cloud:        f.cloud,
		Login:        account.Login{Auth: f.auth, Remote: remote},
		Status:       account.Status{Dir: usecasetest.Dir{DirPath: "/cfg"}, Auth: f.auth, Remote: remote},
		OpenURL:      func(u string) error { f.opened = append(f.opened, u); return nil },
		Wait:         func(_ context.Context, d time.Duration) error { f.waits = append(f.waits, d); return nil },
		RandomSuffix: func() (string, error) { return "abc123", nil },
	}
	return f
}

func (f *setupFixture) withDeny(names ...string) {
	f.policy.Deny = names
	f.u.Init.Policy = f.policy
	f.u.Policy = f.policy
}

func (f *setupFixture) input(projectID, secretPath string) Input {
	return Input{ProjectID: projectID, ClientSecretPath: secretPath, Ask: f.ask}
}

func TestSetup(t *testing.T) {
	t.Run("新しくプロジェクトを作り、API を有効にし、JSON を取り込んでログインする", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.ask.answers = []askAnswer{{val: ""}, {val: "/s.json"}} // ID は Enter（既定値）

		// Act
		got, err := f.u.Run(context.Background(), f.input("", ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		wantCloud := []string{"installed", "account", "describe:" + defaultSetupID, "create:" + defaultSetupID, "enable:" + defaultSetupID}
		if !slices.Equal(f.cloud.calls, wantCloud) {
			t.Errorf("cloud = %v", f.cloud.calls)
		}
		if !slices.Equal(f.auth.Calls, []string{"import", "login"}) || f.auth.Imported != "/s.json" {
			t.Errorf("auth = %v imported=%q", f.auth.Calls, f.auth.Imported)
		}
		if f.ask.defs[0] != defaultSetupID {
			t.Errorf("既定値 = %q", f.ask.defs[0])
		}
		want := Result{ProjectID: defaultSetupID, GCloudAccount: "owner@example.com", ChannelTitle: testChannel.Title, ChannelURL: testChannel.URL(), KeyStore: "os"}
		if got != want {
			t.Errorf("got %+v", got)
		}
		if !f.ask.saidContains("[作成]") || !f.ask.saidContains("やり直すときは `youtube-cli setup --project "+defaultSetupID+"` を使ってください") {
			t.Errorf("said = %v", f.ask.said)
		}
		wantURL := gcp.AuthConsoleURL("overview", defaultSetupID)
		if !slices.Equal(f.opened, []string{wantURL}) {
			t.Errorf("opened = %v", f.opened)
		}
		for _, page := range []string{"branding", "audience", "scopes", "clients"} {
			if !f.ask.saidContains(gcp.AuthConsoleURL(page, defaultSetupID)) {
				t.Errorf("%s の案内がない: %v", page, f.ask.said)
			}
		}
	})

	t.Run("データアクセスの案内に youtube.force-ssl が出る", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.ask.answers = []askAnswer{{val: ""}, {val: "/s.json"}}

		// Act
		_, err := f.u.Run(context.Background(), f.input("", ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if !f.ask.saidContains("youtube.force-ssl") || f.ask.saidContains("drive.file") {
			t.Errorf("said = %v", f.ask.said)
		}
	})

	t.Run("対象の案内は、公開ステータスをテストのままにし、使う人をテストユーザーに足させる", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.ask.answers = []askAnswer{{val: ""}, {val: "/s.json"}}

		// Act
		_, err := f.u.Run(context.Background(), f.input("", ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if !f.ask.saidContains("公開ステータスは「テスト」のままにする") || !f.ask.saidContains("テストユーザー") {
			t.Errorf("said = %v", f.ask.said)
		}
		if f.ask.saidContains("「本番」にする") {
			t.Errorf("本番にさせる案内が残っている: %v", f.ask.said)
		}
	})

	t.Run("プロジェクトが既にあれば作らない", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.cloud.projects["my-proj-1"] = usecase.CloudProject{ID: "my-proj-1", State: "ACTIVE"}

		// Act
		got, err := f.u.Run(context.Background(), f.input("my-proj-1", "/s.json"))

		// Assert
		if err != nil || got.ProjectID != "my-proj-1" {
			t.Fatalf("got %+v, %v", got, err)
		}
		if slices.Contains(f.cloud.calls, "create:my-proj-1") || !slices.Contains(f.cloud.calls, "enable:my-proj-1") {
			t.Errorf("cloud = %v", f.cloud.calls)
		}
		if !f.ask.saidContains("[済み]") {
			t.Errorf("said = %v", f.ask.said)
		}
	})

	t.Run("削除予定のプロジェクトなら undelete の案内で KindSetup", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.cloud.projects["my-proj-1"] = usecase.CloudProject{ID: "my-proj-1", State: "DELETE_REQUESTED"}

		// Act
		_, err := f.u.Run(context.Background(), f.input("my-proj-1", "/s.json"))

		// Assert
		if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), "gcloud projects undelete my-proj-1") {
			t.Errorf("err = %v", err)
		}
		if slices.Contains(f.cloud.calls, "enable:my-proj-1") {
			t.Errorf("cloud = %v", f.cloud.calls)
		}
	})

	t.Run("そのほかの状態なら状態を示して KindSetup", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.cloud.projects["my-proj-1"] = usecase.CloudProject{ID: "my-proj-1", State: "LIFECYCLE_STATE_UNSPECIFIED"}

		// Act
		_, err := f.u.Run(context.Background(), f.input("my-proj-1", "/s.json"))

		// Assert
		if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), "LIFECYCLE_STATE_UNSPECIFIED") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("対話で決めた ID が使われていれば聞き直す", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.cloud.taken[defaultSetupID] = true
		f.auth.SecretProjects["/s.json"] = "my-proj-2"
		f.ask.answers = []askAnswer{{val: ""}, {val: "my-proj-2"}, {val: "/s.json"}}

		// Act
		got, err := f.u.Run(context.Background(), f.input("", ""))

		// Assert
		if err != nil || got.ProjectID != "my-proj-2" {
			t.Fatalf("got %+v, %v", got, err)
		}
		if !f.ask.saidContains("使われて") {
			t.Errorf("said = %v", f.ask.said)
		}
	})

	t.Run("対話で形の違う ID を答えたら理由を出して聞き直す", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretProjects["/s.json"] = "my-proj-2"
		f.ask.answers = []askAnswer{{val: "Bad_ID"}, {val: "my-proj-2"}, {val: "/s.json"}}

		// Act
		got, err := f.u.Run(context.Background(), f.input("", ""))

		// Assert
		if err != nil || got.ProjectID != "my-proj-2" {
			t.Fatalf("got %+v, %v", got, err)
		}
		if !f.ask.saidContains("正しくありません") {
			t.Errorf("said = %v", f.ask.said)
		}
	})

	t.Run("ID の質問で入力が終わったら KindUsage", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.ask.answers = []askAnswer{{err: io.EOF}}

		// Act
		_, err := f.u.Run(context.Background(), f.input("", ""))

		// Assert
		if domain.KindOf(err) != domain.KindUsage || !strings.Contains(err.Error(), "中止しました") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("--project の ID が使われていれば KindConflict", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.cloud.taken["my-proj-1"] = true

		// Act
		_, err := f.u.Run(context.Background(), f.input("my-proj-1", "/s.json"))

		// Assert
		if domain.KindOf(err) != domain.KindConflict {
			t.Errorf("err = %v", err)
		}
		if len(f.ask.asked) != 0 {
			t.Errorf("asked = %v", f.ask.asked)
		}
	})

	t.Run("--project の ID の形が違えば KindUsage で、gcloud を呼ばない", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)

		// Act
		_, err := f.u.Run(context.Background(), f.input("Bad_ID", "/s.json"))

		// Assert
		if domain.KindOf(err) != domain.KindUsage {
			t.Errorf("err = %v", err)
		}
		if len(f.cloud.calls) != 0 {
			t.Errorf("cloud = %v", f.cloud.calls)
		}
	})

	t.Run("gcloud がなければ KindSetup", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.cloud.missing = true

		// Act
		_, err := f.u.Run(context.Background(), f.input("my-proj-1", "/s.json"))

		// Assert
		if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), "https://cloud.google.com/sdk/docs/install") {
			t.Errorf("err = %v", err)
		}
		if !slices.Equal(f.cloud.calls, []string{"installed"}) {
			t.Errorf("cloud = %v", f.cloud.calls)
		}
	})

	t.Run("gcloud にログインしていなければ KindSetup で、プロジェクトに触れない", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.cloud.account = ""

		// Act
		_, err := f.u.Run(context.Background(), f.input("my-proj-1", "/s.json"))

		// Assert
		if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), "gcloud auth login") {
			t.Errorf("err = %v", err)
		}
		if !slices.Equal(f.cloud.calls, []string{"installed", "account"}) {
			t.Errorf("cloud = %v", f.cloud.calls)
		}
	})

	t.Run("API の有効化が失敗してもやり直して成功する", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.cloud.enableFails = 2
		f.cloud.enableErrMsg = "まだ反映されていない"

		// Act
		_, err := f.u.Run(context.Background(), f.input("my-proj-1", "/s.json"))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if f.cloud.enableCalls != 3 || !slices.Equal(f.waits, []time.Duration{2 * time.Second, 4 * time.Second}) {
			t.Errorf("calls=%d waits=%v", f.cloud.enableCalls, f.waits)
		}
	})

	t.Run("API の有効化が5回とも失敗したらエラーを返す", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.cloud.enableFails = 100
		f.cloud.enableErrMsg = "反映されない"

		// Act
		_, err := f.u.Run(context.Background(), f.input("my-proj-1", "/s.json"))

		// Assert
		if err == nil || !strings.Contains(err.Error(), "反映されない") {
			t.Fatalf("err = %v", err)
		}
		want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
		if f.cloud.enableCalls != 5 || !slices.Equal(f.waits, want) {
			t.Errorf("calls=%d waits=%v", f.cloud.enableCalls, f.waits)
		}
		if len(f.auth.Calls) != 0 {
			t.Errorf("auth = %v", f.auth.Calls)
		}
	})

	t.Run("JSON が正しくなければ理由を出して聞き直す", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretErrs["/bad.json"] = domain.Errorf(domain.KindUsage, "形式が正しくありません")
		f.ask.answers = []askAnswer{{val: "/bad.json"}, {val: "/s.json"}}

		// Act
		_, err := f.u.Run(context.Background(), f.input(defaultSetupID, ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if f.auth.Imported != "/s.json" || !f.ask.saidContains("形式が正しくありません") {
			t.Errorf("imported=%q said=%v", f.auth.Imported, f.ask.said)
		}
	})

	t.Run("JSON のパスが空なら KindUsage", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.ask.answers = []askAnswer{{val: "  "}}

		// Act
		_, err := f.u.Run(context.Background(), f.input(defaultSetupID, ""))

		// Assert
		if domain.KindOf(err) != domain.KindUsage || !strings.Contains(err.Error(), "中止しました") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("--client-secret の JSON が正しくなければそのまま返す", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretErrs["/bad.json"] = domain.Errorf(domain.KindUsage, "形式が正しくありません")

		// Act
		_, err := f.u.Run(context.Background(), f.input(defaultSetupID, "/bad.json"))

		// Assert
		if domain.KindOf(err) != domain.KindUsage || len(f.auth.Calls) != 0 {
			t.Errorf("err=%v auth=%v", err, f.auth.Calls)
		}
	})

	t.Run("JSON の project_id が違えば警告し、続けないなら聞き直す", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretProjects["/other.json"] = "other-proj"
		f.ask.answers = []askAnswer{{val: "/other.json"}, {val: "/s.json"}}
		f.ask.confirms = []bool{false}

		// Act
		_, err := f.u.Run(context.Background(), f.input(defaultSetupID, ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if f.auth.Imported != "/s.json" || !f.ask.saidContains("other-proj") || len(f.ask.confirmQ) != 1 {
			t.Errorf("imported=%q said=%v confirm=%v", f.auth.Imported, f.ask.said, f.ask.confirmQ)
		}
	})

	t.Run("JSON の project_id が違っても続けると答えれば取り込む", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretProjects["/other.json"] = "other-proj"
		f.ask.answers = []askAnswer{{val: "/other.json"}}
		f.ask.confirms = []bool{true}

		// Act
		_, err := f.u.Run(context.Background(), f.input(defaultSetupID, ""))

		// Assert
		if err != nil || f.auth.Imported != "/other.json" {
			t.Errorf("err=%v imported=%q", err, f.auth.Imported)
		}
	})

	t.Run("--client-secret の project_id が違い、続けないなら KindUsage", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretProjects["/other.json"] = "other-proj"
		f.ask.confirms = []bool{false}

		// Act
		_, err := f.u.Run(context.Background(), f.input(defaultSetupID, "/other.json"))

		// Assert
		if domain.KindOf(err) != domain.KindUsage || !strings.Contains(err.Error(), "中止しました") || f.auth.Imported != "" {
			t.Errorf("err=%v imported=%q", err, f.auth.Imported)
		}
	})

	t.Run("パスの引用符と ~ を扱う", func(t *testing.T) {
		// Arrange
		home := "/home/u"
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		want := filepath.Join(home, "a b.json")
		for _, answer := range []string{`'~/a b.json'`, `~/a\ b.json`, `"~/a b.json"`, `  ~/a\ b.json  `} {
			f := newSetupFixture(t)
			f.auth.SecretProjects[want] = defaultSetupID
			f.ask.answers = []askAnswer{{val: answer}}

			// Act
			_, err := f.u.Run(context.Background(), f.input(defaultSetupID, ""))

			// Assert
			if err != nil || f.auth.Imported != want {
				t.Errorf("%q: err=%v imported=%q", answer, err, f.auth.Imported)
			}
		}
	})

	t.Run("取り込み済みなら手作業の案内と JSON の質問を飛ばす", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretSaved = true

		// Act
		got, err := f.u.Run(context.Background(), f.input(defaultSetupID, ""))

		// Assert
		if err != nil || got.ChannelTitle != testChannel.Title {
			t.Fatalf("got %+v, %v", got, err)
		}
		if len(f.opened) != 0 || len(f.ask.asked) != 0 || f.ask.saidContains("branding") {
			t.Errorf("opened=%v asked=%v said=%v", f.opened, f.ask.asked, f.ask.said)
		}
		if !slices.Equal(f.auth.Calls, []string{"login"}) || len(f.auth.Checked) != 0 {
			t.Errorf("auth = %v checked=%v", f.auth.Calls, f.auth.Checked)
		}
	})

	t.Run("--client-secret があれば手作業の案内を飛ばす", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)

		// Act
		_, err := f.u.Run(context.Background(), f.input(defaultSetupID, "/s.json"))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if len(f.opened) != 0 || f.ask.saidContains("branding") || len(f.ask.asked) != 0 {
			t.Errorf("opened=%v asked=%v said=%v", f.opened, f.ask.asked, f.ask.said)
		}
		if !slices.Equal(f.auth.Calls, []string{"import", "login"}) {
			t.Errorf("auth = %v", f.auth.Calls)
		}
	})

	t.Run("ログイン済みならログインを飛ばし、status から結果を埋める", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretSaved = true
		f.auth.TokenSaved = true

		// Act
		got, err := f.u.Run(context.Background(), f.input(defaultSetupID, ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if len(f.auth.Calls) != 0 {
			t.Errorf("auth = %v", f.auth.Calls)
		}
		if got.ChannelTitle != testChannel.Title || got.ChannelURL != testChannel.URL() || got.KeyStore != "os" || got.ProjectID != defaultSetupID || got.GCloudAccount != "owner@example.com" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("取り込み直したらログイン済みでもログインする", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.TokenSaved = true

		// Act
		_, err := f.u.Run(context.Background(), f.input(defaultSetupID, "/s.json"))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(f.auth.Calls, []string{"import", "login"}) {
			t.Errorf("auth = %v", f.auth.Calls)
		}
	})

	t.Run("ログインには --no-browser と入出力を渡す", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		in := f.input(defaultSetupID, "/s.json")
		in.NoBrowser = true
		in.Input = strings.NewReader("x")
		var out strings.Builder
		in.Prompt = &out

		// Act
		_, err := f.u.Run(context.Background(), in)
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if !f.auth.Opt.NoBrowser || f.auth.Opt.Input != in.Input || f.auth.Opt.Prompt != in.Prompt {
			t.Errorf("opt = %+v", f.auth.Opt)
		}
	})

	for _, name := range []string{"setup", "login"} {
		t.Run("拒否リストに "+name+" があれば止まり、gcloud を呼ばない", func(t *testing.T) {
			// Arrange
			f := newSetupFixture(t)
			f.withDeny(name)

			// Act
			_, err := f.u.Run(context.Background(), f.input("my-proj-1", "/s.json"))

			// Assert
			if domain.KindOf(err) != domain.KindDenied || !strings.Contains(err.Error(), name) {
				t.Errorf("err = %v", err)
			}
			if len(f.cloud.calls) != 0 || len(f.auth.Calls) != 0 {
				t.Errorf("cloud=%v auth=%v", f.cloud.calls, f.auth.Calls)
			}
		})
	}

	t.Run("鍵の置き場所が使えなければ、GCP に触れる前に止まる", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.InspErr = domain.Errorf(domain.KindSetup, "キーチェーンが使えません")

		// Act
		_, err := f.u.Run(context.Background(), f.input("my-proj-1", "/s.json"))

		// Assert
		if domain.KindOf(err) != domain.KindSetup || !strings.Contains(err.Error(), "キーチェーン") {
			t.Errorf("err = %v", err)
		}
		if len(f.cloud.calls) != 0 {
			t.Errorf("cloud = %v", f.cloud.calls)
		}
	})

	t.Run("--no-browser なら画面を開かない", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		in := f.input(defaultSetupID, "")
		in.NoBrowser = true
		f.ask.answers = []askAnswer{{val: "/s.json"}}

		// Act
		_, err := f.u.Run(context.Background(), in)
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if len(f.opened) != 0 || !f.ask.saidContains("branding") {
			t.Errorf("opened=%v said=%v", f.opened, f.ask.said)
		}
	})

	t.Run("画面を開けなくても続ける", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.u.OpenURL = func(string) error { return errors.New("開けない") }
		f.ask.answers = []askAnswer{{val: "/s.json"}}

		// Act
		_, err := f.u.Run(context.Background(), f.input(defaultSetupID, ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("デスクトップアプリでない（web の）JSON なら理由を出して聞き直す", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretErrs["/web.json"] = domain.Errorf(domain.KindUsage, "client_secret の種類が違います。デスクトップアプリの OAuth クライアントの JSON を指定してください")
		f.ask.answers = []askAnswer{{val: "/web.json"}, {val: "/s.json"}}

		// Act
		_, err := f.u.Run(context.Background(), f.input(defaultSetupID, ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if f.auth.Imported != "/s.json" || !f.ask.saidContains("デスクトップアプリの OAuth クライアントの JSON を指定してください") {
			t.Errorf("imported=%q said=%v", f.auth.Imported, f.ask.said)
		}
	})
}

func TestSetupDefaultProject(t *testing.T) {
	const rerunStored = "やり直すときは `youtube-cli setup --project stored-proj-1` を使ってください"

	t.Run("設定済みの PC で --project なしに Enter だけで再実行すると、保存したプロジェクトを使い、作らず、ログインも飛ばす", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretSaved, f.auth.TokenSaved = true, true
		f.auth.ProjectID = "stored-proj-1"
		f.cloud.projects["stored-proj-1"] = usecase.CloudProject{ID: "stored-proj-1", State: "ACTIVE"}
		f.ask.answers = []askAnswer{{val: ""}}

		// Act
		got, err := f.u.Run(context.Background(), f.input("", ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(f.ask.defs, []string{"stored-proj-1"}) {
			t.Errorf("既定値 = %v", f.ask.defs)
		}
		wantCloud := []string{"installed", "account", "describe:stored-proj-1", "enable:stored-proj-1"}
		if !slices.Equal(f.cloud.calls, wantCloud) {
			t.Errorf("cloud = %v", f.cloud.calls)
		}
		if len(f.auth.Calls) != 0 {
			t.Errorf("auth = %v", f.auth.Calls)
		}
		if got.ProjectID != "stored-proj-1" || got.ChannelTitle != testChannel.Title {
			t.Errorf("got %+v", got)
		}
		if !f.ask.saidContains(rerunStored) {
			t.Errorf("said = %v", f.ask.said)
		}
	})

	t.Run("--client-secret があり --project がなければ、JSON の project_id を既定値にする（保存したものより優先）", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretSaved = true
		f.auth.ProjectID = "stored-proj-1"
		f.auth.SecretProjects["/s.json"] = "json-proj-1"
		f.cloud.projects["json-proj-1"] = usecase.CloudProject{ID: "json-proj-1", State: "ACTIVE"}
		f.ask.answers = []askAnswer{{val: ""}}

		// Act
		got, err := f.u.Run(context.Background(), f.input("", "/s.json"))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(f.ask.defs, []string{"json-proj-1"}) || got.ProjectID != "json-proj-1" {
			t.Errorf("既定値 = %v got=%+v", f.ask.defs, got)
		}
		if slices.ContainsFunc(f.cloud.calls, func(c string) bool { return strings.HasPrefix(c, "create:") }) {
			t.Errorf("cloud = %v", f.cloud.calls)
		}
		if len(f.ask.confirmQ) != 0 || f.auth.Imported != "/s.json" {
			t.Errorf("confirm=%v imported=%q", f.ask.confirmQ, f.auth.Imported)
		}
	})

	for _, projectID := range []string{"", "my-proj-1"} {
		t.Run("--client-secret の JSON が正しくなければ、gcloud を呼ぶ前にそのまま返す（--project="+projectID+"）", func(t *testing.T) {
			// Arrange
			f := newSetupFixture(t)
			f.auth.SecretErrs["/bad.json"] = domain.Errorf(domain.KindUsage, "形式が正しくありません")

			// Act
			_, err := f.u.Run(context.Background(), f.input(projectID, "/bad.json"))

			// Assert
			if domain.KindOf(err) != domain.KindUsage || !strings.Contains(err.Error(), "形式が正しくありません") {
				t.Errorf("err = %v", err)
			}
			if len(f.cloud.calls) != 0 || len(f.ask.asked) != 0 || len(f.auth.Calls) != 0 {
				t.Errorf("cloud=%v asked=%v auth=%v", f.cloud.calls, f.ask.asked, f.auth.Calls)
			}
		})
	}

	t.Run("何も取り込んでいなければ、既定値はランダムな ID", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.ProjectID = "stored-proj-1" // 取り込み済みでなければ使わない
		f.ask.answers = []askAnswer{{val: ""}, {val: "/s.json"}}

		// Act
		_, err := f.u.Run(context.Background(), f.input("", ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if len(f.ask.defs) == 0 || f.ask.defs[0] != defaultSetupID {
			t.Errorf("既定値 = %v", f.ask.defs)
		}
	})

	t.Run("取り込み済みでも project_id が分からなければ、既定値はランダムな ID", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretSaved = true
		f.ask.answers = []askAnswer{{val: ""}}

		// Act
		_, err := f.u.Run(context.Background(), f.input("", ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(f.ask.defs, []string{defaultSetupID}) {
			t.Errorf("既定値 = %v", f.ask.defs)
		}
	})

	t.Run("--project はどの既定値よりも優先し、ID を尋ねない", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretSaved = true
		f.auth.ProjectID = "stored-proj-1"
		f.auth.SecretProjects["/s.json"] = "json-proj-1"
		f.ask.confirms = []bool{true}

		// Act
		got, err := f.u.Run(context.Background(), f.input("my-proj-1", "/s.json"))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if got.ProjectID != "my-proj-1" || len(f.ask.asked) != 0 || !slices.Contains(f.cloud.calls, "describe:my-proj-1") {
			t.Errorf("got=%+v asked=%v cloud=%v", got, f.ask.asked, f.cloud.calls)
		}
		if !f.ask.saidContains("やり直すときは `youtube-cli setup --project my-proj-1` を使ってください") {
			t.Errorf("said = %v", f.ask.said)
		}
	})
}

func TestSetupCancel(t *testing.T) {
	t.Run("ctx が取り消されていたら、gcloud を呼ばずに「中止しました」の KindUsage", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Act
		_, err := f.u.Run(ctx, f.input("my-proj-1", "/s.json"))

		// Assert
		if domain.KindOf(err) != domain.KindUsage || err.Error() != "中止しました" {
			t.Errorf("err = %v", err)
		}
		if len(f.cloud.calls) != 0 || len(f.auth.Calls) != 0 {
			t.Errorf("cloud=%v auth=%v", f.cloud.calls, f.auth.Calls)
		}
	})

	t.Run("gcloud が取り消しで失敗したら、gcloud の失敗ではなく「中止しました」を返す", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f.cloud.onDescribe = func(ctx context.Context) error {
			cancel()
			return domain.Errorf(domain.KindUnexpected, "gcloud projects describe が失敗しました").Wrap(ctx.Err())
		}

		// Act
		_, err := f.u.Run(ctx, f.input("my-proj-1", "/s.json"))

		// Assert
		if domain.KindOf(err) != domain.KindUsage || err.Error() != "中止しました" {
			t.Errorf("err = %v", err)
		}
		if slices.ContainsFunc(f.cloud.calls, func(c string) bool { return strings.HasPrefix(c, "create:") }) {
			t.Errorf("cloud = %v", f.cloud.calls)
		}
	})

	t.Run("API の有効化を待つあいだに取り消されたら「中止しました」で、やり直さない", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f.cloud.enableFails = 100
		f.cloud.enableErrMsg = "まだ反映されていない"
		f.u.Wait = func(ctx context.Context, d time.Duration) error {
			cancel()
			return waitCtx(ctx, d)
		}

		// Act
		_, err := f.u.Run(ctx, f.input("my-proj-1", "/s.json"))

		// Assert
		if domain.KindOf(err) != domain.KindUsage || err.Error() != "中止しました" {
			t.Errorf("err = %v", err)
		}
		if f.cloud.enableCalls != 1 || len(f.auth.Calls) != 0 {
			t.Errorf("enableCalls=%d auth=%v", f.cloud.enableCalls, f.auth.Calls)
		}
	})
}

func TestSetupDeadToken(t *testing.T) {
	failingRemote := func(err error) usecase.Remote {
		return usecase.Remote{
			Open:   func(context.Context) (usecase.YouTube, error) { return nil, err },
			State:  &usecasetest.State{Saved: testChannel, Found: true},
			Locker: &usecasetest.Locker{},
		}
	}

	t.Run("ログイン済みでも status が KindSetup（Google 側でトークンが無効）なら、ログインし直す", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretSaved, f.auth.TokenSaved = true, true
		f.u.Status.Remote = failingRemote(domain.Errorf(domain.KindSetup, "トークンが取り消されています"))

		// Act
		got, err := f.u.Run(context.Background(), f.input(defaultSetupID, ""))
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(f.auth.Calls, []string{"login"}) {
			t.Errorf("auth = %v", f.auth.Calls)
		}
		if got.ChannelTitle != testChannel.Title || got.ChannelURL == "" {
			t.Errorf("got %+v", got)
		}
		if f.ask.saidContains("[省略] ログイン済みです") {
			t.Errorf("said = %v", f.ask.said)
		}
	})

	t.Run("status が KindSetup 以外で失敗したら、ログインせずにそのエラーを返す", func(t *testing.T) {
		// Arrange
		f := newSetupFixture(t)
		f.auth.SecretSaved, f.auth.TokenSaved = true, true
		f.u.Status.Remote = failingRemote(domain.Errorf(domain.KindUnexpected, "通信できません"))

		// Act
		_, err := f.u.Run(context.Background(), f.input(defaultSetupID, ""))

		// Assert
		if domain.KindOf(err) != domain.KindUnexpected || !strings.Contains(err.Error(), "通信できません") {
			t.Errorf("err = %v", err)
		}
		if len(f.auth.Calls) != 0 {
			t.Errorf("auth = %v", f.auth.Calls)
		}
	})
}

func TestNormalizeInputPath(t *testing.T) {
	t.Run("前後の空白と引用符を外し、バックスラッシュ付きの空白を戻し、~ を展開する", func(t *testing.T) {
		// Arrange
		home := "/home/u"
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		cases := map[string]string{
			`  /a/b.json  `:   "/a/b.json",
			`'/a/b c.json'`:   "/a/b c.json",
			`"/a/b c.json"`:   "/a/b c.json",
			`/a/b\ c.json`:    "/a/b c.json",
			`~/x.json`:        filepath.Join(home, "x.json"),
			`'~/x y.json'`:    filepath.Join(home, "x y.json"),
			`~user/x.json`:    "~user/x.json",
			``:                "",
			`'`:               "'",
			`relative/x.json`: "relative/x.json",
			`/c/x`:            "/c/x", // MSYS の形の書き換えは Windows だけ
		}
		for in, want := range cases {
			// Act
			got, err := normalizeInputPath(in, false)

			// Assert
			if err != nil || got != want {
				t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
			}
		}
	})

	t.Run("引用符の外では、バックスラッシュの後ろの1文字をそのまま残す", func(t *testing.T) {
		// Arrange
		home := "/home/u"
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		cases := map[string]string{
			`x\ \(1\).json`:         "x (1).json",
			`/a/b\ \[x\]\&\'y.json`: "/a/b [x]&'y.json",
			`/a/b\\c.json`:          `/a/b\c.json`,
			`~/a\ \(1\).json`:       filepath.Join(home, "a (1).json"),
			`\~/x.json`:             "~/x.json",
			`/a/b\`:                 `/a/b\`,
			`/a/\日本.json`:           "/a/日本.json",
			`'/a/b\ c.json'`:        `/a/b\ c.json`,
			`"/a/(1).json"`:         "/a/(1).json",
		}
		for in, want := range cases {
			// Act
			got, err := normalizeInputPath(in, false)

			// Assert
			if err != nil || got != want {
				t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
			}
		}
	})
	t.Run("Windows では \\ を外さず、/c/ の形を C:/ に直し、~\\ をホームにする", func(t *testing.T) {
		// Arrange
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		cases := map[string]string{
			`C:\Users\me\x.json`:     `C:\Users\me\x.json`,
			`"C:\Users\me\a b.json"`: `C:\Users\me\a b.json`,
			`/c/Users/me/x.json`:     `C:/Users/me/x.json`,
			`~\x.json`:               filepath.Join(home, "x.json"),
			`~/x.json`:               filepath.Join(home, "x.json"),
		}
		for in, want := range cases {
			// Act
			got, err := normalizeInputPath(in, true)

			// Assert
			if err != nil || got != want {
				t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
			}
		}
	})
}

func TestDefaultWait(t *testing.T) {
	t.Run("ctx が取り消されたら待たずに戻る", func(t *testing.T) {
		// Arrange
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Act
		err := waitCtx(ctx, time.Hour)

		// Assert
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v", err)
		}
	})
}
