#!/usr/bin/env bash
#
# Decide the next release version from the commits since the last tag.
#
# Usage: git log --format='-----COMMIT-----%n%s%n%b' <last-tag>..HEAD | next-version.sh <last-tag>
#
# Each commit is a record starting with the marker line: the first line
# after it is the subject, which decides the bump, and the rest is the body,
# which is only searched for breaking-change and skip markers.
#
# Prints the next version (e.g. v0.2.0), or "none" when the commits do not
# warrant a release. With no previous tag it prints the first version.
set -euo pipefail

readonly FIRST_VERSION="v0.1.0"
readonly MARKER="-----COMMIT-----"

last_tag="${1:-}"
input="$(cat)"

if [[ -z "$last_tag" ]]; then
	echo "$FIRST_VERSION"
	exit 0
fi

if [[ ! "$last_tag" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
	echo "next-version: cannot parse last tag '$last_tag'" >&2
	exit 1
fi
major="${BASH_REMATCH[1]}"
minor="${BASH_REMATCH[2]}"
patch="${BASH_REMATCH[3]}"

# An explicit opt-out always wins, wherever it appears.
if grep -qiF '[skip release]' <<<"$input"; then
	echo "none"
	exit 0
fi

bump="none"
raise() {
	case "$1" in
	major) bump="major" ;;
	minor) [[ "$bump" != "major" ]] && bump="minor" ;;
	patch) [[ "$bump" == "none" ]] && bump="patch" ;;
	esac
	return 0
}

classify_subject() {
	local subject="$1"

	# "feat!: ..." and "fix(scope)!: ..." mark a breaking change.
	if [[ "$subject" =~ ^[a-zA-Z]+(\([^\)]*\))?!: ]]; then
		raise major
		return
	fi

	case "$subject" in
	feat:* | feat\(*)
		raise minor
		;;
	fix:* | fix\(* | perf:* | perf\(* | refactor:* | refactor\(* | revert:* | revert\(*)
		raise patch
		;;
	docs:* | docs\(* | chore:* | chore\(* | ci:* | ci\(* | test:* | test\(* | style:* | style\(* | build:* | build\(*)
		# Housekeeping on its own is not worth a release.
		;;
	"")
		;;
	*)
		# A plain commit message still ships something: treat it as a fix.
		raise patch
		;;
	esac
}

expect_subject=false
while IFS= read -r line; do
	if [[ "$line" == "$MARKER" ]]; then
		expect_subject=true
		continue
	fi

	if [[ "$expect_subject" == true ]]; then
		expect_subject=false
		classify_subject "$line"
		continue
	fi

	# Body lines only matter for the breaking-change trailer.
	if [[ "$line" =~ ^BREAKING[\ -]CHANGE ]]; then
		raise major
	fi
done <<<"$input"

case "$bump" in
none)
	echo "none"
	;;
major)
	# Below 1.0.0 a breaking change raises the minor, as semver intends.
	if [[ "$major" -eq 0 ]]; then
		echo "v0.$((minor + 1)).0"
	else
		echo "v$((major + 1)).0.0"
	fi
	;;
minor)
	echo "v$major.$((minor + 1)).0"
	;;
patch)
	echo "v$major.$minor.$((patch + 1))"
	;;
esac
