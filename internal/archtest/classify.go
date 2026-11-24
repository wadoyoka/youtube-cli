// Package archtest は、internal/ の Go のパッケージが層の構成と依存のルールに従っているかを確かめる。
//
// depguard（.golangci.yml）は import を前方一致でしか照合できないので、層の向きはここで確かめる。
// 削除・ゴミ箱・共有の API を本体のコードが呼んでいないかも、ここで確かめる（forbidden.go）。
package archtest

import (
	"slices"
	"strings"
)

// LayerOther は、module の外、internal の外、internal/archtest のパッケージを表す。
const LayerOther = "other"

// layers は internal/ の直下に置ける層。
var layers = []string{"cli", "usecase", "domain", "repository"}

// Classify は、パッケージのパスから、そのパッケージが属する層の名前を返す。
//
// import する側とされる側を同じ物差しで測るため、判断はここ 1 か所にまとめる。
// module の外と internal の外、internal/archtest は LayerOther、internal の中で
// 層の外のものは ""、層の中のサブパッケージ（domain/sub など）はその層にする。
func Classify(modulePath, pkgPath string) string {
	rel, ok := strings.CutPrefix(pkgPath, modulePath+"/")
	if !ok {
		return LayerOther
	}
	return classifyRel(rel)
}

// classifyRel は、module の直下からの相対パス（"internal/usecase" など。区切りは "/"）で
// 判断する。パッケージのパスにも、ディレクトリやファイルのパスにも使う。
func classifyRel(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) < 2 || parts[0] != "internal" || parts[1] == "archtest" {
		return LayerOther
	}
	if slices.Contains(layers, parts[1]) {
		return parts[1]
	}
	return ""
}
