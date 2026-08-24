#!/usr/bin/env bash
#
# Tests for next-version.sh. Run it directly: .github/scripts/next-version_test.sh
set -uo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT="$SCRIPT_DIR/next-version.sh"
readonly MARKER="-----COMMIT-----"

pass=0
fail=0

# build turns "subject|body" pairs into the input the script expects.
build() {
	local c subject body
	for c in "$@"; do
		subject="${c%%|*}"
		body="${c#*|}"
		[[ "$body" == "$c" ]] && body=""
		printf '%s\n%s\n%s\n' "$MARKER" "$subject" "$body"
	done
}

check() {
	local name="$1" tag="$2" want="$3"
	shift 3

	local got
	got="$(build "$@" | "$SCRIPT" "$tag" 2>&1)"
	if [[ "$got" == "$want" ]]; then
		pass=$((pass + 1))
		printf "  ok   %-46s -> %s\n" "$name" "$got"
	else
		fail=$((fail + 1))
		printf "  FAIL %-46s -> %s (want %s)\n" "$name" "$got" "$want"
	fi
}

check "no previous tag"              ""       "v0.1.0"  "anything"
check "feat bumps minor"             "v0.1.0" "v0.2.0"  "feat: add filtering"
check "fix bumps patch"              "v0.1.0" "v0.1.1"  "fix: correct uptime"
check "plain message bumps patch"    "v0.1.0" "v0.1.1"  "Correct the uptime column"
check "docs only: no release"        "v0.1.0" "none"    "docs: fix typo"
check "docs with a body: no release" "v0.1.0" "none"    "docs: readme|Explain the docker integration in detail."
check "chore with prose body"        "v0.1.0" "none"    "chore: bump deps|Updates gopsutil, nothing user visible."
check "chore and ci only"            "v0.1.0" "none"    "chore: bump deps" "ci: cache go modules"
check "feat wins over fix"           "v0.1.0" "v0.2.0"  "fix: a" "feat: b" "fix: c"
check "docs plus feat is a minor"    "v0.1.0" "v0.2.0"  "docs: readme" "feat: b"
check "multiline feat body"          "v0.1.0" "v0.2.0"  "feat: containers|Ask the engine which container owns the port."
check "scoped feat"                  "v0.3.2" "v0.4.0"  "feat(ui): add column"
check "scoped fix"                   "v0.3.2" "v0.3.3"  "fix(docker): retry"
check "breaking below 1.0 is minor"  "v0.4.1" "v0.5.0"  "feat!: drop flag"
check "scoped breaking"              "v0.4.1" "v0.5.0"  "fix(cli)!: rename command"
check "breaking trailer in body"     "v0.4.1" "v0.5.0"  "feat: x|BREAKING CHANGE: the flag is gone"
check "breaking above 1.0 is major"  "v1.4.1" "v2.0.0"  "feat!: drop flag"
check "feat above 1.0"               "v1.4.1" "v1.5.0"  "feat: add"
check "skip release marker"          "v0.1.0" "none"    "feat: big thing|[skip release]"
check "patch rolls over"             "v0.1.9" "v0.1.10" "fix: x"
check "minor resets the patch"       "v0.1.9" "v0.2.0"  "feat: x"
check "no commits"                   "v0.1.0" "none"
check "unparsable tag fails"         "0.1"    "next-version: cannot parse last tag '0.1'" "fix: x"

# The script is fed by git itself, so exercise it against a real repository:
# a format string mistake here cannot be caught by the cases above.
git_integration() {
	local dir
	dir="$(mktemp -d)"
	trap 'rm -rf "$dir"' RETURN

	git -C "$dir" init -q -b main
	git -C "$dir" config user.email test@example.com
	git -C "$dir" config user.name Test

	commit() {
		git -C "$dir" commit -q --allow-empty -m "$1" ${2:+-m "$2"}
	}

	commit "feat: first release"
	local first
	first="$(git -C "$dir" log --format='-----COMMIT-----%n%s%n%b' | "$SCRIPT" "")"
	git -C "$dir" tag v0.1.0

	commit "docs: explain things" "A body that is prose, not a commit type."
	local after_docs
	after_docs="$(git -C "$dir" log --format='-----COMMIT-----%n%s%n%b' v0.1.0..HEAD | "$SCRIPT" v0.1.0)"

	commit "feat(ui): a new column" "With a longer explanation."
	local after_feat
	after_feat="$(git -C "$dir" log --format='-----COMMIT-----%n%s%n%b' v0.1.0..HEAD | "$SCRIPT" v0.1.0)"

	for c in "first:$first:v0.1.0" "docs-only:$after_docs:none" "feat:$after_feat:v0.2.0"; do
		local name="${c%%:*}" rest="${c#*:}"
		local got="${rest%%:*}" want="${rest#*:}"
		if [[ "$got" == "$want" ]]; then
			pass=$((pass + 1))
			printf "  ok   %-46s -> %s\n" "git repo: $name" "$got"
		else
			fail=$((fail + 1))
			printf "  FAIL %-46s -> %s (want %s)\n" "git repo: $name" "$got" "$want"
		fi
	done
}

git_integration

echo
echo "passed: $pass, failed: $fail"
[[ $fail -eq 0 ]]
