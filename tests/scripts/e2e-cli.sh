#!/usr/bin/env bash
# youtube-cli の普段使いのコマンド（status・upload・ls・playlist）が、
# 本物の YouTube で正しく動くかを確かめる。ログイン済みが前提で、login はしない。
#
# 注意: このスクリプトは、本物の YouTube のクォータを使い、youtube-cli では消せない
#       本物の限定公開の動画（YOUTUBE_CLI_E2E_PLAYLISTS=yes のときは再生リストも）を残す。
#       動画のアップロードは 1 本分（1 日 100 本の上限のうちの 1 本）を使う。
#       残ったものは、終わったあとに表示される一覧を見て、YouTube Studio で消す。
#       人が必要なときだけ動かす。Claude などの AI には動かさせない。
#
# 使い方:
#   YOUTUBE_CLI_E2E=yes tests/scripts/e2e-cli.sh                              # PATH の youtube-cli（make install で入れたもの）
#   YOUTUBE_CLI_E2E=yes YOUTUBE_CLI_BIN=./youtube-cli tests/scripts/e2e-cli.sh  # make build で作ったもの
#   YOUTUBE_CLI_E2E=yes YOUTUBE_CLI_E2E_PLAYLISTS=yes tests/scripts/e2e-cli.sh  # 再生リストの確かめも行う（6 つ作る）
#   YOUTUBE_CLI_E2E=yes YOUTUBE_CLI_E2E_THUMBNAIL=yes tests/scripts/e2e-cli.sh  # サムネイルの確かめも行う（チャンネルの電話番号の確認が要る）
#
# - YOUTUBE_CLI_E2E=yes がなければ、何もせずに終了コード 2 で止まる（確認の入力）。
# - 上げる動画は 1 本だけ（ffmpeg で作る数秒の動画。16 MiB より小さいので、1 回の要求で送る経路を確かめる）。
#   タイトルは e2e-cli-<日時>-<PID>。分けて送る経路（16 MiB 以上）はここでは確かめない。
# - YOUTUBE_CLI_E2E_PLAYLISTS=yes のときは、再生リストを 6 つ作る（e2e-cli-<日時>-<PID>-pl1〜pl6）。
#   拒否リストで playlist-create・playlist-add が止まっていれば、先に人が外しておく。
# - 手元には作業場所を mktemp -d で作り、終わったら（失敗や中断でも）消す。
# - 最後の確かめで、設定ディレクトリの ledger.json（この PC の手元の記録）を、一時的に脇へ移して戻す。
#   手元の記録がなくても、YouTube の側から動画と再生リストが引けるかを見るため。それ以外には触れない。
#   YOUTUBE_CLI_CONFIG_DIR・YOUTUBE_CLI_KEYRING はそのまま渡す。
# - 要るもの: bash（3.2 以上）、jq、ffmpeg。
#
# 終了コード: 0 すべて通った / 1 通らなかったものがある / 2 前提が足りない / 130 中断

# macOS の bash 3.2 は、変数のすぐ後の日本語の頭のバイトを変数名の続きとして読む。日本語が続く変数は ${名前} と書く。
# jq の式の中の $w などは jq の変数なので、シェルで展開させない（単引用符のままでよい）。
# shellcheck disable=SC2016
set -u

PASS=0
FAIL=0
# 手元の作業場所。TMP は Git Bash などで export されている名前なので、使わない。
LOCAL_WORK=""
CMD=""
CODE=""
OUT=""
ERR=""
# YouTube に残すもの（終わりに一覧を出す）。
LEFT_VIDEO_ID=""
LEFT_PLAYLISTS=""
# ledger.json を脇へ移しているとき、その場所（finish で戻す）。
LEDGER=""
LEDGER_ASIDE=""

say() { printf '%s\n' "$*"; }

die() {
	printf 'e2e-cli: %s\n' "$*" >&2
	exit 2
}

# --- 確認の入力 ---------------------------------------------------------------

if [ "${YOUTUBE_CLI_E2E:-}" != "yes" ]; then
	cat >&2 <<'MSG'
e2e-cli: このスクリプトは、本物の YouTube のクォータを使い、youtube-cli では消せない
         本物の限定公開の動画（と、再生リスト）を残します。残ったものは、YouTube Studio で消します。
         それでよければ、YOUTUBE_CLI_E2E=yes を付けて動かしてください。
MSG
	exit 2
fi

# --- 前提 -------------------------------------------------------------------

bin_arg="${YOUTUBE_CLI_BIN:-youtube-cli}"
case "$bin_arg" in
*/*)
	if [ ! -f "$bin_arg" ] || [ ! -x "$bin_arg" ]; then
		die "YOUTUBE_CLI_BIN の $bin_arg が見つからないか、実行できません"
	fi
	# 相対パスでも動くよう、絶対パスにしておく。CDPATH があると cd がそちらへ移ってパスを出すので、空にする。
	BIN="$(CDPATH='' cd -- "$(dirname -- "$bin_arg")" && pwd)/$(basename -- "$bin_arg")"
	;;
*)
	BIN="$(command -v "$bin_arg")" || die "$bin_arg が PATH にありません。make install で入れるか、YOUTUBE_CLI_BIN で場所を指定してください"
	;;
esac

command -v jq >/dev/null 2>&1 || die "jq がありません。入れてから動かしてください"
command -v ffmpeg >/dev/null 2>&1 || die "ffmpeg がありません（試験用の動画を作るために使います）。入れてから動かしてください"

# jq_raw は値を文字列で取り出す。Windows の jq は改行を CRLF で出すので、\r を除く。
jq_raw() { jq -r "$@" | tr -d '\r'; }

# --- 後片付け ---------------------------------------------------------------

# trap から呼ぶ。
# shellcheck disable=SC2329
finish() {
	local code=$?
	# 片付けの途中で、もう一度 Ctrl-C されても案内が消えないよう、INT と TERM を無視する。
	trap - EXIT
	trap '' INT TERM
	# 脇へ移した手元の記録を戻す。
	if [ -n "$LEDGER_ASIDE" ] && [ -f "$LEDGER_ASIDE" ]; then
		if [ -e "$LEDGER" ]; then
			say "注意: $LEDGER が新しくできているので、脇へ移した手元の記録を戻せません: $LEDGER_ASIDE"
		else
			mv -- "$LEDGER_ASIDE" "$LEDGER"
		fi
	fi
	# 消すのは、このスクリプトが mktemp -d で作った場所だけ。
	case "$LOCAL_WORK" in
	*/youtube-cli-e2e.*) [ -d "$LOCAL_WORK" ] && rm -rf -- "$LOCAL_WORK" ;;
	esac
	if [ -n "$LEFT_VIDEO_ID" ] || [ -n "$LEFT_PLAYLISTS" ]; then
		say ""
		say "YouTube に、試験で作ったものが残っています。youtube-cli には削除がないので、YouTube Studio で消してください。"
		[ -n "$LEFT_VIDEO_ID" ] && say "  動画: https://studio.youtube.com/video/${LEFT_VIDEO_ID}/edit"
		if [ -n "$LEFT_PLAYLISTS" ]; then
			printf '%s' "$LEFT_PLAYLISTS" | while IFS= read -r line; do
				[ -n "$line" ] && say "  再生リスト: $line"
			done
		fi
	elif [ "${UPLOAD_TRIED:-0}" = 1 ]; then
		# 送る途中で止まったり、応答が届かなかったりしても、YouTube には動画ができていることがある。
		say ""
		say "YouTube に試験の動画（タイトル ${TITLE:-?}）ができているかもしれません。YouTube Studio で確かめて、あれば消してください。"
	fi
	if [ $((PASS + FAIL)) -gt 0 ]; then
		say ""
		say "通った $PASS 件 / 通らなかった $FAIL 件"
	fi
	exit "$code"
}

LOCAL_WORK="$(mktemp -d "${TMPDIR:-/tmp}/youtube-cli-e2e.XXXXXX")" || die "手元の作業場所を作れません"
trap finish EXIT
trap 'exit 130' INT TERM

# --- 確かめの道具 -----------------------------------------------------------

# run は youtube-cli を動かし、CODE・OUT・ERR に結果を入れる。止まらない。
run() {
	CMD="youtube-cli $*"
	"$BIN" "$@" </dev/null >"$LOCAL_WORK/stdout" 2>"$LOCAL_WORK/stderr"
	CODE=$?
	OUT="$(<"$LOCAL_WORK/stdout")"
	ERR="$(<"$LOCAL_WORK/stderr")"
}

ok() {
	PASS=$((PASS + 1))
	say "✓ $1"
}

# ng は、結果を貼ってもらったときに原因を追えるよう、直前のコマンドと出力の頭も出す。
ng() {
	FAIL=$((FAIL + 1))
	say "✗ $1"
	[ $# -ge 2 ] && say "    理由: $2"
	say "    直前のコマンド: $CMD"
	say "    終了コード: $CODE"
	if [ "$CODE" = 7 ]; then
		say "    （終了コード 7 は、拒否リスト policy.toml で止められたことを表す。youtube-cli の不具合ではない）"
	fi
	if [ "$CODE" = 8 ]; then
		say "    （終了コード 8 は、クォータを使い切ったことを表す。太平洋時間の 0 時に戻る）"
	fi
	[ -n "$OUT" ] && say "    標準出力: ${OUT:0:300}"
	if [ -n "$ERR" ]; then
		printf '%s\n' "$ERR" | head -n 3 | sed 's/^/    標準エラー: /'
	fi
}

# expect_json <名前> <jq の式> [jq の引数...]: 標準出力に jq -e を当てる。
expect_json() {
	local name=$1 filter=$2
	shift 2
	if printf '%s' "$OUT" | jq -e "$@" "$filter" >/dev/null 2>&1; then
		ok "$name"
	else
		ng "$name" "jq -e '$filter' が通らない"
	fi
}

# expect_ok <名前> [<jq の式> [jq の引数...]]: 終了コードが 0 で、式があれば標準出力がそれを満たす。
expect_ok() {
	local name=$1
	shift
	if [ "$CODE" != 0 ]; then
		ng "$name" "終了コードが 0 ではない"
	elif [ $# -eq 0 ]; then
		ok "$name"
	else
		expect_json "$name" "$@"
	fi
}

# expect_err_kind <名前> <kind>: 標準エラーの JSON の .error.kind を比べる。
expect_err_kind() {
	if printf '%s' "$ERR" | jq -e --arg k "$2" '.error.kind == $k' >/dev/null 2>&1; then
		ok "$1"
	else
		ng "$1" "標準エラーの .error.kind が $2 ではない"
	fi
}

# expect_err_shape <名前>: 標準エラーが {"error":{"kind","message","details"}} の形かを確かめる。
expect_err_shape() {
	if printf '%s' "$ERR" | jq -e '.error | (.kind | type == "string") and (.message | type == "string") and (.details | type == "array")' >/dev/null 2>&1; then
		ok "$1"
	else
		ng "$1" "標準エラーが {\"error\":{\"kind\",\"message\",\"details\"}} の形ではない"
	fi
}

# expect_error <名前> <終了コード> <kind>: 終了コードと、標準エラーの .error.kind の両方を比べる。
expect_error() {
	if [ "$CODE" != "$2" ]; then
		ng "$1" "終了コードが $2 ではない"
	else
		expect_err_kind "$1" "$3"
	fi
}

# expect_stdout_empty <名前>
expect_stdout_empty() {
	if [ -z "$OUT" ]; then ok "$1"; else ng "$1" "標準出力が空ではない"; fi
}

# --- status ------------------------------------------------------------------

say "youtube-cli: $BIN"
say "手元の作業場所: $LOCAL_WORK"

run status --json
if [ "$CODE" != 0 ] || ! printf '%s' "$OUT" | jq -e '.loggedIn == true' >/dev/null 2>&1; then
	say "youtube-cli の status が通らないので、何も作らずに止めます（ログインしていないか、準備が足りません。status の終了コード: ${CODE}）。"
	[ -n "$ERR" ] && printf '%s\n' "$ERR" | head -n 3 | sed 's/^/  /'
	exit 2
fi
CONFIG_DIR="$(printf '%s' "$OUT" | jq_raw '.configDir // ""')"
say "チャンネル: $(printf '%s' "$OUT" | jq_raw '.channelTitle // ""')（鍵の置き場所: $(printf '%s' "$OUT" | jq_raw '.keyStore // ""')）"
ok "status: ログイン済みと出る"
say ""

# --- 手元の材料 --------------------------------------------------------------

STAMP="e2e-cli-$(date +%Y%m%d-%H%M%S)-$$"
TITLE="$STAMP"
VIDEO="$LOCAL_WORK/$STAMP.mp4"
if ! ffmpeg -loglevel error -f lavfi -i "testsrc=duration=2:size=320x240:rate=10" -pix_fmt yuv420p -c:v libx264 "$VIDEO" 2>"$LOCAL_WORK/ffmpeg.err"; then
	head -n 3 "$LOCAL_WORK/ffmpeg.err" >&2
	die "試験用の動画を作れません（ffmpeg に libx264 が要ります）"
fi
printf 'e2e\n' >"$LOCAL_WORK/desc.txt"

# --- 上げる前に止まるもの（クォータを使わない） -------------------------------

run upload --json "$LOCAL_WORK/none.mp4"
expect_error "upload: 動画のファイルがなければ 4（not_found）" 4 not_found
expect_stdout_empty "upload: 止まったときは、--json でも標準出力に何も出さない"

LONG_TITLE="$(printf 'あ%.0s' $(seq 1 101))"
run upload --json --title "$LONG_TITLE" "$VIDEO"
expect_error "upload: タイトルが 101 文字なら 2（usage）" 2 usage
expect_err_shape "upload: --json のエラーは {\"error\":{\"kind\",\"message\",\"details\"}} の形"

run upload --json --description x --description-file "$LOCAL_WORK/desc.txt" "$VIDEO"
expect_error "upload: --description と --description-file を同時に付けると 2（usage）" 2 usage

run upload --json
expect_error "引数の誤り: upload に引数がなければ 2（usage）" 2 usage

# --- upload ------------------------------------------------------------------

say "（動画を 1 本、本物の YouTube に上げます。少し時間がかかります）"
UPLOAD_TRIED=1
run upload --json --title "$TITLE" --description-file "$LOCAL_WORK/desc.txt" --tag e2e "$VIDEO"
VIDEO_ID="$(printf '%s' "$OUT" | jq_raw '.videoId // ""' 2>/dev/null)"
if [ "$CODE" = 0 ] && [ -n "$VIDEO_ID" ]; then
	LEFT_VIDEO_ID="$VIDEO_ID"
	ok "upload: 動画を上げられる（動画 ID が返る）"
else
	ng "upload: 動画を上げられる（動画 ID が返る）"
	say "動画を上げられないので、ここで止めます。"
	exit 1
fi

# (a) 限定公開で返ってくるか。本物の YouTube でしか確かめられない。
expect_json "upload: 結果が privacyStatus: unlisted（限定公開）で返る" '.privacyStatus == "unlisted" and .existing == false'
expect_json "upload: URL と Studio の URL が動画 ID から作られている" \
	'.url == ("https://youtu.be/" + .videoId) and (.studioUrl | endswith("/" + $id + "/edit"))' --arg id "$VIDEO_ID"
expect_json "upload: 警告がない（限定公開以外で返った警告が出ていない）" '(.warnings | length) == 0'
expect_json "upload: --thumbnail と --playlist がなければ、その欄は null" '.thumbnail == null and .playlist == null'

# --- ls: 上げた直後（手元の記録） ---------------------------------------------

# (b) 上げた直後の動画が、一覧に出るか（YouTube の一覧の遅れを、この PC の手元の記録で補えているか）。
run ls --json
expect_ok "ls: 上げた直後の動画が出る（手元の記録のため）。限定公開で、目印のタグが付いている" \
	'any(.[]; .id == $id and .privacyStatus == "unlisted" and (.tags | index("youtube-cli")) != null and .title == $t)' \
	--arg id "$VIDEO_ID" --arg t "$TITLE"

run ls -l
if [ "$CODE" = 0 ] && printf '%s\n' "$OUT" | grep -q "$VIDEO_ID"; then
	ok "ls -l: 動画 ID つきで一覧を出す"
else
	ng "ls -l: 動画 ID つきで一覧を出す" "終了コードが 0 でないか、動画 ID が出ていない"
fi

# --- 同じタイトル ------------------------------------------------------------

run upload --json --title "$TITLE" "$VIDEO"
expect_error "upload: 同じタイトルをもう一度上げると 5（conflict）" 5 conflict
expect_stdout_empty "upload: 衝突したときは、--json でも標準出力に何も出さない"

run upload --json --title "$TITLE" --skip-existing "$VIDEO"
expect_ok "upload --skip-existing: 上げずに、既にある動画を使う" '.existing == true and .videoId == $id' --arg id "$VIDEO_ID"

run ls --json
expect_ok "upload: 同じタイトルの動画が増えていない" '[.[] | select(.title == $t)] | length == 1' --arg t "$TITLE"

# --- thumbnail（任意） ---------------------------------------------------------

if [ "${YOUTUBE_CLI_E2E_THUMBNAIL:-}" = "yes" ]; then
	THUMB="$LOCAL_WORK/thumb.png"
	if ffmpeg -loglevel error -f lavfi -i "testsrc=duration=1:size=1280x720:rate=1" -frames:v 1 "$THUMB" 2>/dev/null; then
		run thumbnail --json "$VIDEO_ID" "$THUMB"
		expect_ok "thumbnail: サムネイルを設定できる（チャンネルの電話番号の確認が要る）" '.videoId == $id' --arg id "$VIDEO_ID"
	else
		say "注意: サムネイルの画像を作れないので、thumbnail の確かめを飛ばします。"
	fi
fi

# --- playlist（任意） ----------------------------------------------------------

PL_NAMES=""
if [ "${YOUTUBE_CLI_E2E_PLAYLISTS:-}" = "yes" ]; then
	say ""
	say "（再生リストを 6 つ作ります）"
	PL_IDS=""
	PL_CREATED=0
	for n in 1 2 3 4 5 6; do
		name="${STAMP}-pl${n}"
		run playlist create --json "$name"
		pid="$(printf '%s' "$OUT" | jq_raw '.id // ""' 2>/dev/null)"
		if [ "$CODE" = 0 ] && [ -n "$pid" ]; then
			PL_CREATED=$((PL_CREATED + 1))
			PL_NAMES="$PL_NAMES$name
"
			PL_IDS="$PL_IDS$pid
"
			LEFT_PLAYLISTS="$LEFT_PLAYLISTS$name  https://www.youtube.com/playlist?list=${pid}
"
		fi
	done
	if [ "$PL_CREATED" = 6 ]; then
		ok "playlist create: 再生リストを 6 つ作れる"
	else
		ng "playlist create: 再生リストを 6 つ作れる" "作れたのは $PL_CREATED つ（拒否リストで playlist-create が止まっていないか確かめる）"
	fi

	run playlist create --json "${STAMP}-pl1"
	expect_error "playlist create: 同じ名前を作ると 5（conflict）" 5 conflict

	# (c) 6 つ以上の再生リストを、ID で引いて全部見つけられるか。
	#     playlists.list を id で引くとき maxResults を送らなくても、全部返ることの確かめ。
	run playlist ls --json
	expect_ok "playlist ls: 作った 6 つがすべて出る。限定公開で、動画は 0 本" \
		'[.[] | select(.title | startswith($s + "-pl"))] as $mine
		 | ($mine | length == 6) and all($mine[]; .privacyStatus == "unlisted" and .itemCount == 0)' \
		--arg s "$STAMP"

	first_ok=1
	for name in $(printf '%s' "$PL_NAMES"); do
		run ls --playlist "$name" --json
		[ "$CODE" = 0 ] || first_ok=0
	done
	if [ "$first_ok" = 1 ]; then
		ok "ls --playlist: 6 つの再生リストを、名前ですべて見つけられる"
	else
		ng "ls --playlist: 6 つの再生リストを、名前ですべて見つけられる" "見つからないものがある（直前のコマンドの結果を見る）"
	fi

	id_ok=1
	for pid in $(printf '%s' "$PL_IDS"); do
		run ls --playlist "$pid" --json
		[ "$CODE" = 0 ] || id_ok=0
	done
	if [ "$id_ok" = 1 ]; then
		ok "ls --playlist: 6 つの再生リストを、ID ですべて見つけられる"
	else
		ng "ls --playlist: 6 つの再生リストを、ID ですべて見つけられる" "見つからないものがある（直前のコマンドの結果を見る）"
	fi

	PL1="${STAMP}-pl1"
	run playlist add --json "$VIDEO_ID" "$PL1"
	expect_ok "playlist add: 動画を再生リストに入れる" '.added == true and .videoId == $id and .playlistTitle == $p' --arg id "$VIDEO_ID" --arg p "$PL1"

	run playlist add --json "$VIDEO_ID" "$PL1"
	expect_ok "playlist add: すでに入っていれば added が false" '.added == false'

	run ls --playlist "$PL1" --json
	expect_ok "ls --playlist: 入れた動画だけが出る" 'length == 1 and .[0].id == $id' --arg id "$VIDEO_ID"

	run upload --json --title "$TITLE" --skip-existing --playlist "${STAMP}-pl2" "$VIDEO"
	expect_ok "upload --skip-existing --playlist: 上げずに、再生リストへの追加だけを行う" \
		'.existing == true and .playlist.done == true and .thumbnail == null'

	run playlist add --json "no-such-video" "$PL1"
	expect_error "playlist add: このツールで上げていない動画なら 4（not_found）" 4 not_found

	run ls --playlist "no-such-playlist" --json
	expect_error "ls --playlist: ない再生リストなら 4（not_found）" 4 not_found
fi

# --- ls: 手元の記録なし（YouTube の側から） -----------------------------------

# 手元の記録 ledger.json を一時的に脇へ移して、YouTube の側から引いても出るかを見る。
# 上げた直後は一覧に出るのが遅れることがあるので、30 秒おきに最大 10 分待つ。
say ""
LEDGER="$CONFIG_DIR/ledger.json"
if [ -f "$LEDGER" ]; then
	LEDGER_ASIDE="$LEDGER.e2e-aside-$$"
	mv -- "$LEDGER" "$LEDGER_ASIDE" || die "手元の記録を脇へ移せません: $LEDGER"
	say "（手元の記録を脇へ移しました。YouTube の側から動画が引けるまで、最大 10 分待ちます）"
	found=0
	waited=0
	while [ "$waited" -le 600 ]; do
		run ls --json
		if [ "$CODE" = 0 ] && printf '%s' "$OUT" | jq -e --arg id "$VIDEO_ID" 'any(.[]; .id == $id)' >/dev/null 2>&1; then
			found=1
			break
		fi
		sleep 30
		waited=$((waited + 30))
	done
	if [ "$found" = 1 ]; then
		ok "ls: 手元の記録がなくても、YouTube の側から動画が出る（待ち ${waited} 秒）"
	else
		ng "ls: 手元の記録がなくても、YouTube の側から動画が出る" "10 分待っても出ない"
	fi
	if [ -n "$PL_NAMES" ]; then
		run playlist ls --json
		expect_ok "playlist ls: 手元の記録がなくても、6 つがすべて出る" \
			'[.[] | select(.title | startswith($s + "-pl"))] | length == 6' --arg s "$STAMP"
	fi
	mv -- "$LEDGER_ASIDE" "$LEDGER" && LEDGER_ASIDE=""
else
	say "注意: 手元の記録（$LEDGER）がないので、YouTube の側から引く確かめを飛ばします。"
fi

# --- 引数の誤り --------------------------------------------------------------

run upload --json
expect_stdout_empty "引数の誤り: --json のエラーは標準エラーに出て、標準出力は空"

[ "$FAIL" -eq 0 ] || exit 1
exit 0
