package youtubeapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

// mapError は、API のエラーを domain.Error に変える。エラーの文字列は利用者に見せるので、URL の問い合わせ部分は取り除く。
func mapError(err error) error {
	if err == nil {
		return nil
	}
	redactURL(err)
	if _, ok := errors.AsType[*domain.Error](err); ok {
		return err
	}
	if _, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
		return domain.Errorf(domain.KindSetup, "ログインが切れています。`youtube-cli login` を実行してください").Wrap(err)
	}
	if gerr, ok := errors.AsType[*googleapi.Error](err); ok {
		switch {
		case isQuotaExceeded(gerr):
			return domain.Errorf(domain.KindQuota, "YouTube API の割り当て（クォータ）を使い切りました。太平洋時間の 0 時に戻ります").Wrap(err)
		case gerr.Code == http.StatusNotFound:
			return domain.Errorf(domain.KindNotFound, "YouTube に見つかりません").Wrap(err)
		}
	}
	return domain.Errorf(domain.KindUnexpected, "YouTube の操作に失敗しました").Wrap(err)
}

// isQuotaExceeded は、1日の割り当てやアップロード数の上限を使い切った失敗（403）かどうかを返す。
// 短い間の混み合い（rateLimitExceeded など）は含まない。それはやり直せば通る。
func isQuotaExceeded(gerr *googleapi.Error) bool {
	if gerr.Code != http.StatusForbidden {
		return false
	}
	for _, e := range gerr.Errors {
		switch e.Reason {
		case "quotaExceeded", "uploadLimitExceeded", "dailyLimitExceeded":
			return true
		}
	}
	return false
}

// redactURL は、err の連なりにある url.Error の URL から、問い合わせ部分を取り除く。
// 再開可能アップロードの upload_id のように、他人に知られると困る値が入ることがあるため。
func redactURL(err error) {
	if uerr, ok := errors.AsType[*url.Error](err); ok {
		if i := strings.IndexByte(uerr.URL, '?'); i >= 0 {
			uerr.URL = uerr.URL[:i]
		}
	}
}
