#!/usr/bin/env bash
#
# スキル 1 つ分のセットアップスクリプト
#
#   このリポジトリ内のスキル本体を、Claude Code / Codex から参照できるように
#   ホーム配下へシンボリックリンクで接続する。何度実行しても同じ結果になる（冪等）。
#   サブエージェントの定義（agents/claude/<name>.md・agents/codex/<name>.toml）があれば、それも接続する。
#
# 使い方:
#   ./setup.sh                # Claude Code + Codex（Codex は導入済みのときだけ）
#   ./setup.sh --claude-only  # Claude Code だけ
#   ./setup.sh --codex-only   # Codex だけ
#   ./setup.sh --with-codex   # Codex 未導入でも配置する
#   ./setup.sh --dry-run      # 何もせず、実行される内容だけ表示
#   ./setup.sh --uninstall    # このリポジトリを指すリンクを取り外す
#   ./setup.sh --help
#
set -euo pipefail

# ---- パス解決（リポジトリをどこに置いても動く） -----------------------------
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
SKILL_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd -P)"
SKILL_NAME="$(basename -- "$SKILL_DIR")"

# Claude Code: スキルは ~/.claude/skills/、サブエージェントは ~/.claude/agents/*.md
CLAUDE_SKILL_LINK="$HOME/.claude/skills/$SKILL_NAME"
CLAUDE_AGENT_LINK="$HOME/.claude/agents/$SKILL_NAME.md"
CLAUDE_AGENT_SRC="$SKILL_DIR/agents/claude/$SKILL_NAME.md"

# Codex: スキルは ~/.agents/skills/ を探索する（USER レベルの探索パス）。
#        サブエージェントは ~/.codex/agents/*.toml。
CODEX_SKILL_LINK="$HOME/.agents/skills/$SKILL_NAME"
CODEX_AGENT_LINK="$HOME/.codex/agents/$SKILL_NAME.toml"
CODEX_AGENT_SRC="$SKILL_DIR/agents/codex/$SKILL_NAME.toml"

# 退避先。~/.claude/skills/ や ~/.agents/skills/ の中に残すと、
# .bak ディレクトリまで重複スキルとして読み込まれてしまうため、探索対象外へ逃がす。
BACKUP_DIR="$HOME/.agent-skills-backups"

# ---- オプション -------------------------------------------------------------
DO_CLAUDE=1
DO_CODEX=1
FORCE_CODEX=0
DRY_RUN=0
UNINSTALL=0

usage() { sed -n '3,17p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

while [ $# -gt 0 ]; do
  case "$1" in
    --claude-only) DO_CODEX=0 ;;
    --codex-only)  DO_CLAUDE=0; FORCE_CODEX=1 ;;
    --with-codex)  FORCE_CODEX=1 ;;
    --dry-run)     DRY_RUN=1 ;;
    --uninstall)   UNINSTALL=1 ;;
    -h|--help)     usage; exit 0 ;;
    *) echo "不明なオプション: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

# ---- 表示ヘルパ -------------------------------------------------------------
if [ -t 1 ]; then B=$'\033[1m'; G=$'\033[32m'; Y=$'\033[33m'; R=$'\033[31m'; D=$'\033[2m'; N=$'\033[0m'
else B=; G=; Y=; R=; D=; N=; fi

ok()    { printf '  %s✓%s %s\n' "$G" "$N" "$*"; }
skip()  { printf '  %s-%s %s\n' "$D" "$N" "$*"; }
warn()  { printf '  %s!%s %s\n' "$Y" "$N" "$*"; }
err()   { printf '  %s✗%s %s\n' "$R" "$N" "$*" >&2; }
head_() { printf '\n%s%s%s\n' "$B" "$*" "$N"; }
note()  { printf '  %s%s%s\n' "$D" "$*" "$N"; }
run()   { if [ "$DRY_RUN" = 1 ]; then printf '  %s$ %s%s\n' "$D" "$*" "$N"; else "$@"; fi; }

# パスをホーム相対の見やすい形に
tilde() { case "$1" in "$HOME"/*) printf '~%s' "${1#"$HOME"}" ;; *) printf '%s' "$1" ;; esac; }

# 既存の実体を退避する（シンボリックリンクでない場合のみ）。
# 退避名は「どこにあったか」で修飾する（例: claude-skills--youtube-cli.bak.<日時>）。
backup() {
  local path="$1"
  local dir; dir="$(dirname -- "$path")"
  local tag; tag="$(basename -- "$(dirname -- "$dir")")-$(basename -- "$dir")"
  tag="${tag#.}"
  local stamp; stamp="$(date +%Y%m%d-%H%M%S)"
  local bak="$BACKUP_DIR/$tag--$(basename -- "$path").bak.$stamp"
  local n=2
  while [ -e "$bak" ]; do bak="$BACKUP_DIR/$tag--$(basename -- "$path").bak.$stamp-$n"; n=$((n + 1)); done
  warn "既存の実体を退避: $(tilde "$path") → $(tilde "$bak")"
  run mkdir -p -- "$BACKUP_DIR"
  run mv -- "$path" "$bak"
}

# $2 を指すシンボリックリンクを $1 に作る
link() {
  local dst="$1" src="$2"
  run mkdir -p -- "$(dirname -- "$dst")"
  if [ -L "$dst" ]; then
    if [ "$(readlink -- "$dst")" = "$src" ]; then
      skip "既にリンク済み: $(tilde "$dst")"
      return 0
    fi
    warn "別の場所を指すリンクを差し替え: $(tilde "$dst") → $(readlink -- "$dst")"
    run rm -f -- "$dst"
  elif [ -e "$dst" ]; then
    backup "$dst"
  fi
  run ln -s -- "$src" "$dst"
  ok "$(tilde "$dst") → $(tilde "$src")"
}

# このリポジトリを指しているときだけ取り外す
unlink_if_ours() {
  local dst="$1" src="$2"
  if [ -L "$dst" ] && [ "$(readlink -- "$dst")" = "$src" ]; then
    run rm -f -- "$dst"
    ok "取り外し: $(tilde "$dst")"
  elif [ -e "$dst" ] || [ -L "$dst" ]; then
    skip "このリポジトリのリンクではないので残す: $(tilde "$dst")"
  else
    skip "未インストール: $(tilde "$dst")"
  fi
}

codex_present() { command -v codex >/dev/null 2>&1 || [ -d "$HOME/.codex" ]; }

# ---- 事前チェック -----------------------------------------------------------
[ -f "$SKILL_DIR/SKILL.md" ] || { err "必要なファイルが見つかりません: $SKILL_DIR/SKILL.md"; exit 1; }

printf '%sスキル:%s %s\n' "$B" "$N" "$(tilde "$SKILL_DIR")"
[ "$DRY_RUN" = 1 ] && printf '%s(dry-run: 実際には何も変更しません)%s\n' "$Y" "$N"

# ---- アンインストール -------------------------------------------------------
if [ "$UNINSTALL" = 1 ]; then
  head_ "Claude Code"
  unlink_if_ours "$CLAUDE_SKILL_LINK" "$SKILL_DIR"
  unlink_if_ours "$CLAUDE_AGENT_LINK" "$CLAUDE_AGENT_SRC"

  head_ "Codex"
  unlink_if_ours "$CODEX_SKILL_LINK" "$SKILL_DIR"
  unlink_if_ours "$CODEX_AGENT_LINK" "$CODEX_AGENT_SRC"

  printf '\n%s完了。%s 退避ファイル（%s）は自動では戻しません。\n' "$G" "$N" "$(tilde "$BACKUP_DIR")"
  exit 0
fi

# ---- Claude Code ------------------------------------------------------------
if [ "$DO_CLAUDE" = 1 ]; then
  head_ "Claude Code"
  link "$CLAUDE_SKILL_LINK" "$SKILL_DIR"
  if [ -f "$CLAUDE_AGENT_SRC" ]; then
    link "$CLAUDE_AGENT_LINK" "$CLAUDE_AGENT_SRC"
    note "スキルは自動発火。/$SKILL_NAME で呼べる。サブエージェントは $SKILL_NAME という名前で委譲できる"
  else
    note "スキルは自動発火。/$SKILL_NAME で呼べる"
  fi
fi

# ---- Codex ------------------------------------------------------------------
if [ "$DO_CODEX" = 1 ]; then
  head_ "Codex"
  if ! codex_present && [ "$FORCE_CODEX" != 1 ]; then
    skip "Codex が見つかりません。導入後に ./setup.sh を再実行してください"
    skip "今すぐ配置したい場合は --with-codex を付けてください"
  else
    # Codex は ~/.agents/skills/ をスキルの探索パスとして走査するので、リンクを張るだけでよい。
    link "$CODEX_SKILL_LINK" "$SKILL_DIR"
    if [ -f "$CODEX_AGENT_SRC" ]; then
      # Codex はファイル名ではなく、TOML の name フィールドでサブエージェントを識別する。
      link "$CODEX_AGENT_LINK" "$CODEX_AGENT_SRC"
      agent_name="$(sed -n 's/^[[:space:]]*name[[:space:]]*=[[:space:]]*"\(.*\)".*/\1/p' "$CODEX_AGENT_SRC" | head -1)"
      note "スキルは自動発火。サブエージェントは名前 ${agent_name:-?} で委譲できる"
    else
      note "スキルは自動発火"
    fi
  fi
fi

printf '\n%s完了。%s\n' "$G" "$N"
