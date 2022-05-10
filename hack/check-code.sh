#!/bin/bash
arg="${1:-""}"
if [ "$arg" == "" ]; then
    echo "Plesae input the base branch name you checkout from"
    echo "Usage: ./check-code.sh <base-branch-name>"
    echo "eg: if you checkout branch from master, ./check-code.sh master"
    echo "    if you checkout branch from release-4.10, ./check-code.sh release-4.10"
    exit 2
fi

head=$(git rev-parse --short HEAD | xargs echo -n)
set +e
modified_files_check=""
modified_files=$(git diff-tree --no-commit-id --name-only -r $arg..$head | \
	grep "^test" | grep ".go$" | grep -v "bindata.go$" | grep -v "Godeps" | \
	grep -v "third_party")
if [ -n "${modified_files}" ]; then
    for f in $modified_files;
    do
        if [ -e $f ]; then
            modified_files_check="$modified_files_check $f";
        fi 
    done
    echo -e "Checking modified files: ${modified_files_check}\n"
fi
set -e

echo -e "\n###############  golint  ####################"
bad_golint_files=""
if [ -n "${modified_files_check}" ]; then
    bad_golint_files=$(echo $modified_files_check | xargs -n1 golint)
fi

if [[ -n "${bad_golint_files}" ]]; then
    echo "ERROR:"
	echo "golint detected following problems:"
	echo "${bad_golint_files}"
else
    echo "golint SUCCESS"
fi

echo -e "\n###############  gofmt  ####################"
bad_gofmt_files=$(echo $modified_files_check | xargs gofmt -s -l)
if [[ -n "${bad_gofmt_files}" ]]; then
	echo "ERROR:"
    echo "!!! gofmt needs to be run on the listed files"
	echo "${bad_gofmt_files}"
	echo "Try running 'gofmt -s [file_path]' Or autocorrect with 'gofmt -s -w [file_path]'"
else
    echo "gofmt SUCCESS"
fi

if [[ -n "${bad_golint_files}" ]]||[[ -n "${bad_gofmt_files}" ]]; then
	exit 1
fi
