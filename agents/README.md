# agents

youtube-cli を AI エージェント（Claude Code・Codex）から使うためのスキルの置き場。

スキルの実体はこのリポジトリで管理し、`setup.sh` が `~/.claude/` `~/.agents/` `~/.codex/` へ
シンボリックリンクを張って接続する。ホーム配下にコピーは置かない。
`SKILL.md` は Claude Code と Codex が同じ形式で読む（`name` / `description` の frontmatter が必須）ので、スキル本体は 1 つで足りる。

## 収録スキル

| スキル | 概要 |
|--------|------|
| [youtube-cli](skills/youtube-cli/) | youtube-cli で YouTube へ限定公開の動画を上げ、サムネイル・再生リスト・一覧を扱う。ログインの確かめ方、`--json` の読み方、終了コードごとの止まり方、タイトルの付け方（`【動画シリーズ名】#.{No} 動画タイトル`）を決めてある |

## セットアップ

先に、youtube-cli を入れてログインまで済ませておく（[リポジトリの README](../README.md)）。スキルは `setup` や `login` を実行しない。

```bash
cd <このリポジトリ>/agents
./setup.sh
```

| コマンド | 動作 |
|----------|------|
| `./setup.sh` | 全スキルを Claude Code / Codex に接続（Codex は導入済みのときだけ） |
| `./setup.sh --dry-run` | 変更せず、実行される内容だけ表示 |
| `./setup.sh --claude-only` | Claude Code だけ |
| `./setup.sh --codex-only` | Codex だけ |
| `./setup.sh --with-codex` | Codex 未導入でも配置 |
| `./setup.sh --uninstall` | このリポジトリを指すリンクを取り外す |
| `./setup.sh <スキル名>` | 対象スキルだけ処理（オプションと併用可） |

接続はすべてシンボリックリンクなので、リポジトリ側を編集する（`git pull` する）と、そのまま反映される。
リポジトリを動かしたら、`./setup.sh` を実行し直す（古いリンクを差し替える）。

既に同じ名前の実体（リンクでないファイルやディレクトリ）がある場合は、`~/.agent-skills-backups/` に
`<出所>--<名前>.bak.<日時>` として退避してから張り替える。`~/.claude/skills/` の中に残すと、
`.bak` まで重複スキルとして読み込まれるため、外へ逃がしている。

| | Claude Code | Codex |
|---|---|---|
| スキルの設置先 | `~/.claude/skills/<name>` | `~/.agents/skills/<name>` |
| サブエージェントの設置先（あれば） | `~/.claude/agents/<name>.md` | `~/.codex/agents/<name>.toml` |
| 呼び出し | 自動発火 / `/<name>` | 自動発火 |

## AI エージェントに使わせる前に

- 自分で 1 回 `youtube-cli status` を実行する。macOS で youtube-cli を入れ直した直後は、キーチェーンのダイアログが出るので、人が許可しておく（[docs/setup.md](../docs/setup.md#鍵の置き場所)）。
- させたくない操作は、拒否リスト（`policy.toml`）で止める（[docs/policy.md](../docs/policy.md)）。初期値では、再生リストの作成と追加（`playlist-create`・`playlist-add`）が止まっている。エージェントに再生リストを使わせるときは、人がこの 2 つを外す。
- OAuth スコープ（`youtube.force-ssl`）は広いので、設定ディレクトリのファイル（トークンと鍵）をエージェントに渡さない（[docs/limitations.md](../docs/limitations.md)）。
- エージェントから使うときの決まりの詳細は [docs/ai-agents.md](../docs/ai-agents.md)。

## ディレクトリ構成

```
agents/
├── setup.sh                 全スキル一括セットアップ（各 skills/*/scripts/setup.sh を呼ぶ）
├── README.md
└── skills/
    └── youtube-cli/
        ├── SKILL.md         スキル本体（Claude Code / Codex 共通）
        ├── references/      コマンドの早見表（JSON の形、終了コード）
        └── scripts/setup.sh このスキルの接続スクリプト
```

## スキルを追加するには

1. `skills/<name>/` を作り、`SKILL.md`（`name` と `description` の frontmatter 必須。`name` はディレクトリ名と同じ）を置く。
2. サブエージェントも配るなら、`skills/<name>/agents/claude/<name>.md`（Markdown + YAML frontmatter）と
   `skills/<name>/agents/codex/<name>.toml`（`name` / `description` / `developer_instructions` が必須）を置く。なければスキルだけを接続する。
3. `skills/<name>/scripts/setup.sh` を既存スキルからコピーする（自分の位置からスキル名・パスを解決するので、そのままで動く）。
4. `./setup.sh <name>` で接続する。
