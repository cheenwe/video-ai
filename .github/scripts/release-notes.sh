#!/usr/bin/env bash
# 输出某个 tag（如 v0.2.0）的发版说明正文。
# 优先取 CHANGELOG.md 中 "## <version>" 段落；没有对应条目时输出占位说明，
# GitHub 自动生成的变更对照（--generate-notes）会追加在正文之后。
set -euo pipefail
tag="${1:?usage: release-notes.sh <vX.Y.Z>}"
version="${tag#v}"
file="CHANGELOG.md"

if [[ ! -f "$file" ]]; then
  printf '## %s\n\n仓库暂无 CHANGELOG.md；变更对照见下方自动生成内容。\n' "$version"
  exit 0
fi

# 匹配二级标题 "## <version>"（版本后必须紧跟空格/制表符/行尾），
# 打印到下一个二级标题之前。
section=$(awk -v ver="$version" '
  !found && $1 == "##" && $2 == ver { found = 1; print; next }
  found && /^## / { exit }
  found { print }
' "$file")

if [[ -n "$section" ]]; then
  printf '%s\n' "$section"
else
  printf '## %s\n\nCHANGELOG.md 中暂无 %s 条目（发布前请按 Keep a Changelog 约定补写）；变更对照见下方自动生成内容。\n' "$version" "$version"
fi
