#!/bin/sh
#
# Cut a release by tagging one.
#
# The version lives in the git tag and nowhere else: goreleaser reads it from
# there and stamps it into the binary, so there is no file to bump and a
# release is a tag and a push. This works out the next tag, refuses to cut one
# from a tree that is not fit to release, and leaves the building to CI.
#
# Called from the Makefile - `make release-patch` and friends - rather than by
# hand, though it runs on its own just as well.

set -eu

BUMP=${1:-patch}
REMOTE=${REMOTE:-origin}
BRANCH=${BRANCH:-main}

usage() {
	echo "usage: $0 [patch|minor|major|vX.Y.Z]" >&2
	exit 2
}

die() {
	echo "$0: $1" >&2
	exit 1
}

case "$BUMP" in
patch | minor | major) ;;
v[0-9]*) ;;
*) usage ;;
esac

# The checks come before anything is computed, so a tree that cannot be
# released says so rather than printing a version it is not going to cut.
[ -n "$(git status --porcelain)" ] &&
	die "the working tree has changes - commit or stash them first"

current_branch=$(git rev-parse --abbrev-ref HEAD)
if [ "$current_branch" != "$BRANCH" ] && [ "${ANY_BRANCH:-}" != "1" ]; then
	die "on $current_branch, not $BRANCH - pass ANY_BRANCH=1 to release from here anyway"
fi

# Tags are fetched too: the next version is worked out from them, and a stale
# view of them would hand back a version somebody has already released.
git fetch --quiet --tags "$REMOTE" "$BRANCH" ||
	die "cannot reach $REMOTE - a release has to be pushed, so it has to be reachable"

if [ "$(git rev-parse HEAD)" != "$(git rev-parse FETCH_HEAD)" ]; then
	die "$BRANCH is not in step with $REMOTE - push or pull first, so the tag lands on what everyone else has"
fi

# The highest version there has ever been, rather than the nearest tag behind
# HEAD: `git describe` answers with whichever tag it reaches first, which is
# not the same question when several sit on one commit or a fix was tagged
# later on an older branch. A version number is never issued twice.
latest=$(git tag --list 'v*' --sort=-v:refname | head -n 1)
[ -n "$latest" ] || latest="v0.0.0"

if [ "${BUMP#v}" != "$BUMP" ]; then
	next=$BUMP
else
	# The oldest tag here has no leading v, so it is stripped rather than
	# assumed, and a version with fewer than three parts fills in zeros.
	numbers=${latest#v}
	major=$(echo "$numbers" | cut -d. -f1)
	minor=$(echo "$numbers" | cut -d. -f2)
	patch=$(echo "$numbers" | cut -d. -f3)

	[ -n "$major" ] || major=0
	[ -n "$minor" ] || minor=0
	[ -n "$patch" ] || patch=0

	case "$BUMP" in
	major) major=$((major + 1)); minor=0; patch=0 ;;
	minor) minor=$((minor + 1)); patch=0 ;;
	patch) patch=$((patch + 1)) ;;
	esac

	next="v$major.$minor.$patch"
fi

git rev-parse -q --verify "refs/tags/$next" >/dev/null &&
	die "$next is already tagged"

echo "Releasing $latest -> $next from $(git rev-parse --short HEAD) on $current_branch"

# The same tests the release workflow runs, run before the tag rather than
# after it: a tag that fails CI is one that has to be deleted from a remote
# people may already have fetched.
if [ "${SKIP_TESTS:-}" != "1" ]; then
	echo "Testing..."
	go test ./... >/dev/null || die "the tests fail - not releasing"
fi

# Pushing a tag starts a public release, so it is asked about rather than
# assumed. YES=1 is for a script that has already asked somebody.
if [ "${YES:-}" != "1" ]; then
	printf "Tag and push %s? [y/N] " "$next"
	read -r answer
	case "$answer" in
	y | Y | yes | YES) ;;
	*) die "nothing was tagged" ;;
	esac
fi

git tag -a "$next" -m "$next"
git push --quiet "$REMOTE" "refs/tags/$next"

echo "Pushed $next. The release workflow builds it from here:"
echo "  https://github.com/fefeme/workingon/actions"
