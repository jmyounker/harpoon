#!/bin/bash
#
# check-style ensures that all packages pass go fmt and go vet.
#
# This differs from the pre-commit hook which: 1) doesn't run on CI, and 2)
# only checks changed files.

function checkfmt() {
  unformatted=$(go fmt -n ./... | sed 's/ -w//' | bash)
  [ -z "$unformatted" ] && return 0

  echo >&2 "Go files must be formatted with gofmt. Please run:"
  for fn in $unformatted; do
    echo >&2 "  gofmt -w $PWD/$fn"
  done

  return 1
}

function checkvet() {
  unvetted=$(go vet ./... 2>&1 | grep -v "exit status 1")
  [ -z "$unvetted" ] && return 0

  echo >&2 "Go files must be vetted. Check these problems:"
  IFS=$'\n'
  for line in $unvetted; do
    echo >&2 "  $line"
  done
  unset IFS

  return 1
}

checkfmt || fail=yes
checkvet || fail=yes

[ -z "$fail" ] || exit 1

exit 0
