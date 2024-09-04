# encoding: utf-8
#!/usr/bin/env python3
import re, os, sys, time, subprocess

# get the updated content
# ToDo: to infer the test case ID when the test case content updates, not the title update
# `git show master..` can get all the updated commits content
commitAuthor = os.popen('git log -n 1 --pretty=format:"%an"', 'r').read()
print("author is ", commitAuthor)
print("get updated files under test/extended")
if commitAuthor == "ci-robot":
    commitStr=os.popen('git log -n 1 --pretty=format:"%p"', 'r').read()
    commit1 = commitStr.split()[0]
    commit2 = os.popen('git log -n 1 --pretty=format:"%h"', 'r').read()
else:
    commit1="master"
    commit2= os.popen('git rev-parse --short HEAD | xargs echo -n', 'r').read()
commands = 'git diff-tree --no-commit-id --name-only -r '+commit1+' '+commit2 +' |grep "^test" | grep ".go$" | grep -v "bindata.go$" | grep -v "third_party" | grep -v "test/extended/testdata"'
print (commands)
process = subprocess.Popen(commands, shell=True, stdout=subprocess.PIPE)
process.wait()
modifedFiles, _ = process.communicate()
print(modifedFiles.decode("utf-8").strip(os.linesep))
if not modifedFiles:
    print("no go file is modified")
    sys.exit(0)

caseList=[]
patternIt = re.compile('.*\-(\d{5,})\-')
for filename in modifedFiles.decode("utf-8").strip(os.linesep).split():
    print("Search the updated cases for "+filename)
    diffcommands = 'git diff '+commit1 +' ' +commit2 + ' -- ' + filename.strip(os.linesep) +'|grep g.It > /tmp/git_diff.log'
    print(diffcommands)
    process = subprocess.Popen(diffcommands, shell=True, stdout=subprocess.PIPE)
    try:
        outs, errs = process.communicate(timeout=300)
    except subprocess.TimeoutExpired:
        process.kill()
        raise Exception(diffcommands +" timeout")

    print ("=======updated content========")
    with open("/tmp/git_diff.log", 'r', encoding='utf-8') as content:
        for line in content:
            lineIndex = str(line)
            print(lineIndex)
            if "g.It" not in lineIndex:
                continue
            if "#" in lineIndex:
                continue
            if not lineIndex.startswith("+"):
                continue
            if re.search("Flaky|VMonly|NonUnifyCI｜CPaasrunOnly｜ProdrunOnly｜StagerunOnly|DisconnectedOnly|HyperShiftMGMT|MicroShiftOnly|ChkUpgrade", lineIndex):
                continue
            if "\"" not in lineIndex:
                continue
            caseString = lineIndex.split("\"")[1]
            caseIDs = patternIt.findall(caseString)
            for caseID in caseIDs:
                if caseID not in caseList:
                    caseList.append(caseID)
    print ("=======updated content========")
    
print(caseList)
if caseList:
    testcaseIDs = "|".join(caseList)
    #Skip Security_and_Compliance cases, cannot executed these cases now.
    commands = './bin/extended-platform-tests run all --dry-run |grep -E "'+ testcaseIDs +'"'
    process = subprocess.Popen(commands, shell=True, stdout=subprocess.PIPE)
    cases, err = process.communicate()
    if patternIt.findall(str(cases)):
        # run test case
        commands = './bin/extended-platform-tests run all --dry-run |grep -E "'+ testcaseIDs + '" |./bin/extended-platform-tests run --timeout 45m -f -'
        print (commands)
        process = subprocess.Popen(commands, shell=True, stdout=subprocess.PIPE)
        out, err = process.communicate()
        print("output:")
        for line in str(out).split("\\n"):
            print(line)
        if err:
            for line in str(err).split("\\n"):
                print(line)
        if process.returncode != 0:
            raise Exception(commands +" failed")
    else:
        print ("Skip Security_and_Compliance cases, cannot executed these cases now")
else:
    print ("There is no Test Case found")    
