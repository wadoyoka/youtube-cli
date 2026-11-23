package archtest

import (
	"fmt"
	"slices"
	"strings"
)

// allowList は、ある層が import してよい層。表に無い組み合わせはすべて許さない。
// 同じ層（usecase から usecase/sub など）も許す。層が大きくなったらサブパッケージに分けるため。
var allowList = map[string][]string{
	"cli":        {"cli", "usecase", "domain"},
	"usecase":    {"usecase", "domain"},
	"domain":     {"domain"},
	"repository": {"repository", "usecase", "domain"},
}

// CheckImport は、from の層が to の層を import してよいかを確かめる。許されないときは、
// 理由を日本語で返す。
//
// from と to のどちらかが LayerOther（main、標準ライブラリ、外部のライブラリなど）なら
// 確かめない。どちらかが ""（internal の中の層の外）ならエラーにする。
func CheckImport(from, to string) error {
	if from == LayerOther || to == LayerOther {
		return nil
	}
	if from == "" || to == "" {
		return fmt.Errorf("%s から %s は import できません。層の外のパッケージは、%s のどれかのディレクトリに移してください", describe(from), describe(to), strings.Join(layers, "・"))
	}
	if slices.Contains(allowList[from], to) {
		return nil
	}
	hint := ""
	if from == "cli" && to == "repository" {
		hint = "。repository の実装を usecase に渡すのは main.go の役目です"
	}
	return fmt.Errorf("%s から %s は import できません（%s が import してよいのは、%s です）%s", describe(from), describe(to), from, strings.Join(allowList[from], "・"), hint)
}

// describe は層をエラーの文面に入れる形にする。
func describe(layer string) string {
	if layer == "" {
		return "internal の層の外のパッケージ"
	}
	return "internal/" + layer
}
