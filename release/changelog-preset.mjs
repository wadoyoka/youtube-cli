/**
 * semantic-release の commit-analyzer と release-notes-generator が使う preset。
 * release.config.mjs から `config: "./release/changelog-preset.mjs"` で読む。
 *
 * preset を名前（preset: "conventionalcommits"）で指定すると、両プラグインはまず自分の置き場所から探す。
 * pnpm ではそこから commitlint が持つ別の版の preset が見えることがあり、その版は
 * release-notes-generator の conventional-changelog-writer と合わずに失敗する。
 * ここで直に import して、package.json に書いた版を使わせる。
 */
import createPreset from "conventional-changelog-conventionalcommits";

// リリースノートに載せる type と、その見出し。載せない type は書かない。
const types = [
  { type: "feat", section: "新機能" },
  { type: "fix", section: "不具合の修正" },
  { type: "perf", section: "速さの改善" },
  { type: "revert", section: "取り消し" },
];

export default function changelogPreset() {
  return createPreset({ types });
}
