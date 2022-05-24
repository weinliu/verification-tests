# encoding: utf-8
#!/usr/bin/env python3
import re, os, time, subprocess

# get the updated content
# ToDo: to infer the test case ID when the test case content updates, not the title update
# `git show master..` can get all the updated commits content
commitAuthor = os.popen('git log -n 1 --pretty=format:"%an"', 'r').read()
print("author is ", commitAuthor)
if commitAuthor == "ci-robot":
    commitStr=os.popen('git log -n 1 --pretty=format:"%p"', 'r').read()
    commit1 = commitStr.split()[0]
    commit2 = os.popen('git log -n 1 --pretty=format:"%h"', 'r').read()
    commands = 'git diff '+commit1+' '+commit2
else:
    commands = 'git diff master..'
print (commands)
process = subprocess.Popen(commands, shell=True, stdout=subprocess.PIPE)
process.wait()
content, _ = process.communicate()
# content = '''
# +    g.It("Medium-34883-SDK stamp on Operator bundle image", func() {
# +   g.It("ConnectedOnly-High-37826-Low-23170-Medium-20979-High-37442-use an PullSecret for the private Catalog Source image [Serial]", func() {

print ("=======updated content========")
for line in str(content).split("\\n"):
    print(line)
print ("=======updated content========")

#time.sleep(10800)
# get the test case IDs
print("Search the updated cases")
caseList=[]
patternIt = re.compile('.*\-(\d{5,})\-')
for lineIndex in str(content).split("\\n"):
    if "g.It" not in lineIndex:
        continue
    if "#" in lineIndex:
        continue
    if not lineIndex.startswith("+"):
        continue
    if re.search("VMonly|NonUnifyCI｜CPaasrunOnly｜ProdrunOnly｜StagerunOnly", lineIndex):
        continue
    caseString = lineIndex.split("\"")[1]
    caseIDs = patternIt.findall(caseString)
    for caseID in caseIDs:
        if caseID not in caseList:
            caseList.append(caseID)
print(caseList)
if caseList:
    testcaseIDs = "|".join(caseList)
    #Skip Security_and_Compliance cases, cannot executed these cases now.
    commands = './bin/extended-platform-tests run all --dry-run |grep -E "'+ testcaseIDs +'" |grep -v Security_and_Compliance'
    process = subprocess.Popen(commands, shell=True, stdout=subprocess.PIPE)
    cases, err = process.communicate()
    if patternIt.findall(str(cases)):
        # run test case
        commands = './bin/extended-platform-tests run all --dry-run |grep -E "'+ testcaseIDs + '" |grep -v Security_and_Compliance |./bin/extended-platform-tests run -f -'
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
