/**
 * semantic-release の設定。
 *
 * dev を main にマージすると、.github/workflows/release.yml がこれで動く。
 * 前のタグからのコミットの type で、次の版を決める。
 *   feat → マイナー版を上げる（1.2.3 → 1.3.0）
 *   fix・perf・revert → パッチ版を上げる（1.2.3 → 1.2.4）
 *   本文に「BREAKING CHANGE:」がある → メジャー版を上げる（1.2.3 → 2.0.0）
 *   それ以外（docs・refactor・test・build・ci・chore など）だけなら、リリースしない
 *
 * 作るのはタグ（v1.2.3）と GitHub のリリース（リリースノート）だけ。実行ファイルのビルドと配布はしない。
 * 版を書き込むファイルはないので、版のコミットも作らない（Go のモジュールはタグが版になる）。
 *
 * v2 以降のタグを Go のモジュールとして使ってもらうには、go.mod の module を …/v2 にしておく必要がある。
 * BREAKING CHANGE を入れるときは、先に module のパスを変える。
 */
import commitlint from "./commitlint.config.mjs";

// preset は名前ではなく、このファイルで渡す（理由は changelog-preset.mjs の先頭）。
const config = "./release/changelog-preset.mjs";

// コミットメッセージを commitlint と同じ形（type: :emoji: 件名）で読む。
// commitlint が通したコミットは、ここでも同じように読める。
const parserOpts = commitlint.parserPreset.parserOpts;

export default {
  branches: ["main"],
  plugins: [
    [
      "@semantic-release/commit-analyzer",
      {
        config,
        parserOpts,
        // 既定で上がるのは feat・fix・perf と BREAKING CHANGE だけ。
        // このリポジトリでは取り消しも type（revert: :rewind: …）で書くので、パッチ版を上げる。
        releaseRules: [{ type: "revert", release: "patch" }],
      },
    ],
    ["@semantic-release/release-notes-generator", { config, parserOpts }],
    [
      "@semantic-release/github",
      {
        // 載せる実行ファイルや人が書き直す手順はないので、下書きにせずそのまま公開する。
        releaseNameTemplate: "youtube-cli v<%= nextRelease.version %>",
        // PR や issue にコメントやラベルを付けない（ラベルはコメントと同じ分岐にあり、一緒に止まる）。
        // 失敗しても issue を作らない。どちらも止めると、ワークフローの権限は contents: write だけで足りる。
        successCommentCondition: false,
        failCommentCondition: false,
      },
    ],
  ],
};
