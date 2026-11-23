package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

type errorBody struct {
	Kind    string   `json:"kind"`
	Message string   `json:"message"`
	Details []string `json:"details"`
}

// writeJSON は、v を JSON として 1 行で書く。
func writeJSON(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}

// writeError は、エラーを標準エラーに書く。domain.Error に原因（Err）があれば、補足の最後の行として出す。
// ただし、本文や補足に同じ文字列が既にあれば、重ねて出さない。
func writeError(w io.Writer, asJSON bool, err error) {
	body := errorBody{Kind: domain.KindOf(err).String(), Message: err.Error(), Details: []string{}}
	if de, ok := errors.AsType[*domain.Error](err); ok {
		body.Message = de.Message
		body.Details = append(body.Details, de.Details...)
		if de.Err != nil {
			if cause := de.Err.Error(); cause != body.Message && !slices.Contains(body.Details, cause) {
				body.Details = append(body.Details, cause)
			}
		}
	}
	if asJSON {
		_ = writeJSON(w, map[string]errorBody{"error": body})
		return
	}
	_, _ = fmt.Fprintf(w, "エラー: %s\n", body.Message)
	for _, d := range body.Details {
		_, _ = fmt.Fprintf(w, "  - %s\n", d)
	}
}
