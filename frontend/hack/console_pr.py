# encoding: utf-8
#!/usr/bin/env python3
import re
import os
import sys
import time
import subprocess

# get the updated content
# `git show master..` can get all the updated commits content
commitAuthor = os.popen('git log -n 1 --pretty=format:"%an"', 'r').read()
print("author is ", commitAuthor)
print("get updated files under frontend/tests")
if commitAuthor == "ci-robot":
    commitStr = os.popen('git log -n 1 --pretty=format:"%p"', 'r').read()
    commit1 = commitStr.split()[0]
    commit2 = os.popen('git log -n 1 --pretty=format:"%h"', 'r').read()
else:
    commit1 = "master"
    commit2 = os.popen(
        'git rev-parse --short HEAD | xargs echo -n', 'r').read()
commands = 'git diff-tree --no-commit-id --name-only -r '+commit1+' '+commit2 + \
    ' |grep "^frontend"'
print(commands)
process = subprocess.Popen(commands, shell=True, stdout=subprocess.PIPE)
process.wait()
modifiedFiles, _ = process.communicate()
print(modifiedFiles.decode("utf-8").strip(os.linesep))
if not modifiedFiles:
    print("no files are modified")
    sys.exit(0)

fileChangedPattern = re.compile('^frontend/(tests.*)')
modifiedTests = []
for filename in modifiedFiles.decode("utf-8").strip(os.linesep).split():
    match = fileChangedPattern.match(filename)
    if match:
        modifiedTests.append(match.groups()[0])

if not modifiedTests:
    print("no tests are modified")
    sys.exit(0)

print("====== Modified Tests are =======")
print(*modifiedTests, sep="\n")

if len(modifiedTests) > 1:
    testsToRun = ','.join(modifiedTests)
else:
    testsToRun = modifiedTests[0]

commands = 'cd frontend; ./console-test-frontend.sh --spec ' + testsToRun
process = subprocess.Popen(commands, shell=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
while process.poll() is None:
    nextline = process.stdout.readline()
    sys.stdout.write((nextline.decode("utf-8")))
    sys.stdout.flush()
out, err = process.communicate()
if process.returncode != 0:
    raise Exception(commands + " failed")
