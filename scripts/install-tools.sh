#!/bin/sh
# このリポジトリが前提にしている開発ツールを入れる。
# バージョンは Makefile が持っていて、CI も同じ値を使う。
set -eu

GOLANGCI_LINT_VERSION=${GOLANGCI_LINT_VERSION:-v2.13.1}
GITLEAKS_VERSION=${GITLEAKS_VERSION:-v8.30.1}

wanted=${*:-}
[ -n "$wanted" ] || wanted="golangci-lint gitleaks"

gobin=$(go env GOBIN)
[ -n "$gobin" ] || gobin="$(go env GOPATH)/bin"

want() {
	for name in $wanted; do
		[ "$name" = "$1" ] && return 0
	done
	return 1
}

# PATH が go install の出力先とは別の実行ファイルを指していると、入れ替えたつもりでも
# 古いほうが動き続ける。lint の結果が CI と食い違う原因になるので知らせる。
warn_if_shadowed() {
	resolved=$(command -v "$1" 2>/dev/null || true)
	if [ -n "$resolved" ] && [ "$resolved" != "$gobin/$1" ]; then
		echo "    警告: PATH では $resolved が優先される。CI と違う結果になりうる。"
	fi
}

echo "開発ツールを確認する"

if want golangci-lint; then
	current=$("$gobin/golangci-lint" --version 2>/dev/null | sed -n 's/.*has version \([0-9.]*\).*/v\1/p')
	if [ "$current" = "$GOLANGCI_LINT_VERSION" ]; then
		echo "  golangci-lint: $GOLANGCI_LINT_VERSION（そのまま）"
	else
		if [ -z "$current" ]; then
			echo "  golangci-lint: なし → $GOLANGCI_LINT_VERSION を入れる"
		else
			echo "  golangci-lint: $current → $GOLANGCI_LINT_VERSION に入れ替える"
		fi
		go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$GOLANGCI_LINT_VERSION"
	fi
	warn_if_shadowed golangci-lint
fi

if want gitleaks; then
	# go install で入れた gitleaks はバージョンを名乗らない（リリース時に埋め込まれる値のため）。
	# 入っているかどうかだけを見る。入れ直したいときは実行ファイルを消してから実行する。
	if [ -x "$gobin/gitleaks" ]; then
		echo "  gitleaks: 導入済み（バージョンは確認できない）"
	else
		echo "  gitleaks: なし → $GITLEAKS_VERSION を入れる"
		go install "github.com/zricethezav/gitleaks/v8@$GITLEAKS_VERSION"
	fi
	warn_if_shadowed gitleaks
fi

case ":$PATH:" in
*":$gobin:"*) ;;
*) echo "  警告: $gobin が PATH に入っていない。shell の設定に追加すること。" ;;
esac

echo "完了"
