#!/bin/bash
arg="${1:-""}"
if [ "$arg" == "" ]; then
    echo "Plesae input the base branch name you checkout from"
    echo "Usage: ./check-code.sh <base-branch-name>"
    echo "eg: if you checkout branch from master, ./check-code.sh master"
    echo "    if you checkout branch from release-4.10, ./check-code.sh release-4.10"
    exit 2
fi
set +e
commit1=""
commit2=""
commit_log=$(git log -n 1 --pretty=format:"%an")
if [[ $commit_log == "ci-robot" ]]; then
  commit1=$(git log -n 1 --pretty=format:"%p" | awk '{print $1}')
  commit2=$(git log -n 1 --pretty=format:"%h")
else
  commit1=$arg
  commit2=$(git rev-parse --short HEAD | xargs echo -n)
fi
if [[ "x${commit1}x" == "xx" ]] || [[ "x${commit2}x" == "xx" ]];then
    echo "get commit id failed"
    exit 1
fi

echo "run 'git diff-tree --no-commit-id --name-only -r $commit1..$commit2'"
modified_files_check=""
modified_files=$(git diff-tree --no-commit-id --name-only -r $commit1..$commit2 | \
	grep "^test" | grep ".go$" | grep -v "bindata.go$" | grep -v "Godeps" | \
	grep -v "third_party" | grep -v "test/extended/testdata")
if [ -n "${modified_files}" ]; then
    for f in $modified_files;
    do
        if [ -e $f ]; then
            modified_files_check="$modified_files_check $f";
        fi 
    done
    echo -e "Checking modified files: ${modified_files_check}\n"
else
    git diff-tree --no-commit-id --name-only -r $commit1..$commit2
    echo -e "no go file is modified"
    exit 0
fi

set -e
echo -e "\n###############  golint  ####################"
bad_golint_files=""
# if [ -n "${modified_files_check}" ]; then
#     bad_golint_files=$(echo $modified_files_check | xargs -n1 golint)
# fi

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

echo -e "\n###############  ginkgo version check  ####################"
bad_ginkgover_files=""
bad_ginkgorep_files=""
echo -e "$modified_files"
for f in $modified_files;
do
    if [ -e $f ]; then
        version_check_result=$(echo $f | xargs grep 'github.com/onsi/ginkgo' | grep -v 'github.com/onsi/ginkgo/v2' || true)
        if [[ -n "${version_check_result}" ]]; then
            bad_ginkgover_files="$bad_ginkgover_files $f";
        fi
        version_check_result=$(echo $f | xargs grep 'CurrentGinkgoTestDescription' || true)
        if [[ -n "${version_check_result}" ]]; then
            bad_ginkgorep_files="$bad_ginkgorep_files $f";
        fi
    fi 
done

if [[ -n "${bad_ginkgover_files}" ]]||[[ -n "${bad_ginkgorep_files}" ]]; then
    echo -e "from 4.12, please use github.com/onsi/ginkgo/v2 and CurrentSpecReport, not github.com/onsi/ginkgo and CurrentGinkgoTestDescription"
    echo -e "before 4.12, please use github.com/onsi/ginkgo and CurrentGinkgoTestDescription, not github.com/onsi/ginkgo/v2 and CurrentSpecReport"
    echo "ERROR:"
	echo "ginkgo version check detected following problems:"
    test -n "${bad_ginkgover_files}" && echo "please use github.com/onsi/ginkgo/v2 in ${bad_ginkgover_files}" || true
    test -n "${bad_ginkgorep_files}" && echo "please use CurrentSpecReport in ${bad_ginkgorep_files}" || true
else
    echo "ginkgo version check SUCCESS"
fi

if [[ -n "${bad_ginkgover_files}" ]]||[[ -n "${bad_ginkgorep_files}" ]]; then
	exit 1
fi

if [[ -n "${bad_golint_files}" ]]||[[ -n "${bad_gofmt_files}" ]]; then
	exit 1
fi
