package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

func TestWriteError(t *testing.T) {
	cause := errors.New("googleapi: Error 503: backend error")

	t.Run("原因のあるエラーは、原因を補足の行として出す", func(t *testing.T) {
		// Arrange
		err := domain.Errorf(domain.KindUnexpected, "YouTube の操作に失敗しました").Wrap(cause)
		var buf bytes.Buffer

		// Act
		writeError(&buf, false, err)

		// Assert
		want := "エラー: YouTube の操作に失敗しました\n  - googleapi: Error 503: backend error\n"
		if buf.String() != want {
			t.Errorf("got %q, want %q", buf.String(), want)
		}
	})

	t.Run("--json でも、原因を details に入れる", func(t *testing.T) {
		// Arrange
		err := domain.Errorf(domain.KindNotFound, "YouTube に見つかりません").WithDetails("/a.txt").Wrap(cause)
		var buf bytes.Buffer

		// Act
		writeError(&buf, true, err)

		// Assert
		var got struct {
			Error errorBody `json:"error"`
		}
		if jerr := json.Unmarshal(buf.Bytes(), &got); jerr != nil {
			t.Fatalf("json: %v: %q", jerr, buf.String())
		}
		want := []string{"/a.txt", "googleapi: Error 503: backend error"}
		if got.Error.Kind != "not_found" || got.Error.Message != "YouTube に見つかりません" || !slices.Equal(got.Error.Details, want) {
			t.Errorf("got %+v", got.Error)
		}
	})

	t.Run("本文や補足と同じ原因は、重ねて出さない", func(t *testing.T) {
		// Arrange
		sameAsMessage := domain.Errorf(domain.KindUsage, "%s", cause.Error()).Wrap(cause)
		inDetails := domain.Errorf(domain.KindDenied, "policy.toml を読めません").WithDetails(cause.Error()).Wrap(cause)
		var buf1, buf2 bytes.Buffer

		// Act
		writeError(&buf1, false, sameAsMessage)
		writeError(&buf2, false, inDetails)

		// Assert
		if strings.Count(buf1.String(), cause.Error()) != 1 {
			t.Errorf("buf1 = %q", buf1.String())
		}
		if strings.Count(buf2.String(), cause.Error()) != 1 {
			t.Errorf("buf2 = %q", buf2.String())
		}
	})

	t.Run("原因のないエラーは、補足を足さない", func(t *testing.T) {
		// Arrange
		err := domain.Errorf(domain.KindConflict, "/a は既にあります")
		var buf bytes.Buffer

		// Act
		writeError(&buf, true, err)

		// Assert
		if want := `{"error":{"kind":"conflict","message":"/a は既にあります","details":[]}}` + "\n"; buf.String() != want {
			t.Errorf("got %q, want %q", buf.String(), want)
		}
	})
}
