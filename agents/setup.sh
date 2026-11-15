#!/usr/bin/env bash
#
# youtube-cli のエージェントスキルを一括でセットアップする
#
#   skills/*/scripts/setup.sh をすべて実行し、各スキルを
#   Claude Code / Codex に接続する。オプションはそのまま各スクリプトへ渡す。
#
# 使い方:
#   ./setup.sh                      # 全スキルをセットアップ
#   ./setup.sh --dry-run            # 変更せず内容だけ表示
#   ./setup.sh --uninstall          # 全スキルを取り外す
#   ./setup.sh youtube-cli           # スキル名を指定
#   ./setup.sh youtube-cli --dry-run
#
set -euo pipefail

AGENTS_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"

if [ -t 1 ]; then B=$'\033[1m'; Y=$'\033[33m'; N=$'\033[0m'; else B=; Y=; N=; fi

# 非オプション引数をスキル名として拾う
TARGETS=()
ARGS=()
for a in "$@"; do
  case "$a" in
    -*) ARGS+=("$a") ;;
    *)  TARGETS+=("$a") ;;
  esac
done

if [ "${#TARGETS[@]}" -eq 0 ]; then
  for d in "$AGENTS_DIR"/skills/*/; do
    [ -x "$d/scripts/setup.sh" ] && TARGETS+=("$(basename -- "$d")")
  done
fi

if [ "${#TARGETS[@]}" -eq 0 ]; then
  echo "セットアップ対象のスキルが見つかりません（skills/*/scripts/setup.sh）" >&2
  exit 1
fi

status=0
for name in "${TARGETS[@]}"; do
  script="$AGENTS_DIR/skills/$name/scripts/setup.sh"
  if [ ! -x "$script" ]; then
    printf '%s! %s: scripts/setup.sh がありません（スキップ）%s\n' "$Y" "$name" "$N" >&2
    status=1
    continue
  fi
  printf '\n%s══ %s ══%s\n' "$B" "$name" "$N"
  "$script" ${ARGS[@]+"${ARGS[@]}"} || status=1
done

exit "$status"
