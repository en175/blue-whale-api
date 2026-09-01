#!/usr/bin/env bash
# Commit a task worktree diff as a reviewable, revertable checkpoint.
# worktree 工单分支上的 commit 是本地可逆动作（不碰 master、不 push、不 PR），
# 不属发布红线，无需用户授权门槛。用户确认环节落在 merge 本地 master 前。
#
# 两种模式：
#   提交（默认）：把已完成的子 issue diff 落成一个新 commit。一个工单分支可以提交多次——
#            一个父工单一个 worktree、逐子项各落一个 commit 是既定流程，不是例外。
#   --amend：用户 review commit 后发现修订，覆盖最后 commit。默认 --no-edit 保留原 commit
#            message；-m 覆盖 message。仅在该分支已有自己的 commit 后可用。

set -euo pipefail

MESSAGE=""
AMEND=0

usage() {
  cat >&2 <<EOF
用法:
  提交:            scripts/commit-task-for-review.sh -m "<commit-message>"
  review 后修订:   scripts/commit-task-for-review.sh --amend [-m "<new-message>"]

在任务 worktree 工单分支内提交已完成的子 issue diff，落成可 review、可 revert 的检查点。
同一分支可反复提交（逐子项一个 commit）。--amend 覆盖该分支最后 commit（默认保留原
message，-m 覆盖）；仅在该分支已有自己的 commit 后可用。
EOF
  exit 2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    -m|--message)
      [ "$#" -ge 2 ] || usage
      MESSAGE="$2"
      shift 2
      ;;
    --amend)
      AMEND=1
      shift
      ;;
    -h|--help)
      usage
      ;;
    *)
      echo "❌ commit-task-for-review: 不支持的参数：$1" >&2
      usage
      ;;
  esac
done

fail() {
  echo "❌ commit-task-for-review: $*" >&2
  exit 1
}

ROOT="$(git rev-parse --show-toplevel)"
# Git Bash on Windows：git rev-parse 返回 `D:/...`（大写盘符），pwd 返回 `/d/...`（小写 posix）。
# case 比较大小写敏感会误判，统一走 pwd 风格。
ROOT="$(cd "$ROOT" && pwd)"
# shellcheck source=lib/worktree-paths.sh
source "$ROOT/scripts/lib/worktree-paths.sh"
WORKTREE_BASE="$(worktree_paths_resolve_base "$ROOT")"

case "$ROOT" in
  "$WORKTREE_BASE"/*) ;;
  *) fail "请在 $WORKTREE_BASE 下的任务 worktree 内执行，不能在用户本地 master 直接提交任务。" ;;
esac

branch="$(git -C "$ROOT" symbolic-ref --short HEAD 2>/dev/null || true)"
case "$branch" in
  feature/*|fix/*|hotfix/*|docs/*|chore/*) ;;
  "") fail "当前不是分支状态。" ;;
  *) fail "当前分支不是工单分支：$branch" ;;
esac

# amend 的唯一真实前提是「这个分支上已经有属于它自己的 commit」，否则 amend 会改写 worktree
# 基线（分出来时的 master SHA）。以前用一个标记文件表示这件事，副作用是把「一个分支只能提交
# 一次」也一起钉死了；改用向 master 求 merge-base 直接问 git，既准确又不再限制提交次数。
base_ref=""
for candidate in origin/master master; do
  if git -C "$ROOT" rev-parse --verify --quiet "$candidate" >/dev/null; then
    base_ref="$candidate"
    break
  fi
done
[ -n "$base_ref" ] || fail "找不到 master 基线引用（origin/master 或 master），无法判断分支是否已有自己的 commit。"

branch_has_own_commit() {
  local base
  base="$(git -C "$ROOT" merge-base "$base_ref" HEAD 2>/dev/null || true)"
  [ -n "$base" ] || return 1
  [ "$base" != "$(git -C "$ROOT" rev-parse HEAD)" ]
}

if [ "$AMEND" = 1 ]; then
  branch_has_own_commit || fail "当前工单分支尚无自己的 commit，--amend 会改写 worktree 基线。请先用 -m \"<message>\" 提交。"
  git -C "$ROOT" add -A
  if [ -n "$MESSAGE" ]; then
    git -C "$ROOT" commit --amend -m "$MESSAGE"
  else
    git -C "$ROOT" commit --amend --no-edit
  fi
  echo "✅ 已修订任务验收分支的最后 commit：$branch"
else
  [ -n "$MESSAGE" ] || fail "缺少 commit message。请用 -m \"<message>\"。"
  if [ -z "$(git -C "$ROOT" status --porcelain)" ]; then
    fail "没有可提交的任务验收 diff。"
  fi
  git -C "$ROOT" add -A
  git -C "$ROOT" commit -m "$MESSAGE"
  commit_count="$(git -C "$ROOT" rev-list --count "$base_ref"..HEAD)"
  # ${} 包起来：紧跟其后的全角括号是多字节字符，裸 $branch 会被 bash 当成变量名的一部分。
  echo "✅ 已提交任务验收分支：${branch}（本分支第 ${commit_count} 个 commit）"
fi

echo "下一步：用户确认后，可在本地 master 运行 scripts/merge-task-to-local-master.sh --user-authorized $branch"
