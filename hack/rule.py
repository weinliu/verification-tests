# encoding: utf-8
#!/usr/bin/env python3
import re, os, sys, subprocess

subteam = [
            "SDN","STORAGE","Developer_Experience","User_Interface","PerfScale", "Service_Development_B","NODE","LOGGING","Logging",
            "Workloads","Metering","Cluster_Observability","Quay/Quay.io","Cluster_Infrastructure",
            "Multi-Cluster","Cluster_Operator","Azure","Network_Edge","ETCD","INSTALLER","Portfolio_Integration",
            "Service_Development_A","OLM","Operator_SDK","App_Migration","Windows_Containers","Security_and_Compliance",
            "KNI","Openshift_Jenkins","RHV","ISV_Operators","PSAP","Multi-Cluster-Networking","OTA","Kata","Build_API",
            "Image_Registry","Container_Engine_Tools","MCO","API_Server","Authentication","Hypershift","Network_Observability",
            "DR_Testing","OAP","User_Interface_Cypress","Insights","Sample", "Cluster_Management_Service"
        ]

sigs = [
    "sig-api-machinery",
    "sig-apps",
    "sig-auth",
    "sig-baremetal",
    "sig-cco",
    "sig-cli",
    "sig-cluster-lifecycle",
    "sig-disasterrecovery",
    "sig-etcd",
    "sig-hive",
    "sig-hypershift",
    "sig-imageregistry",
    "sig-isc",
    "sig-kata",
    "sig-mco",
    "sig-monitoring",
    "sig-netobserv",
    "sig-network",
    "sig-network-edge",
    "sig-networking",
    "sig-node",
    "sig-tuning-node",   
    "sig-openshift-logging",
    "sig-operators",
    "sig-perfscale",
    "sig-rosacli",
    "sig-scheduling",
    "sig-storage",
    "sig-updates",
    "sig-windows",
    "sig-oap"
]

frameworkLabels = [
    "DisconnectedOnly",
    "ConnectedOnly",
    "OSD_NONCCS",
    "OSD_CCS",
    "ARO",
    "ROSA",
    "HyperShiftMGMT",
    "NonHyperShiftHOST",
    "StressTest",
    "MicroShiftOnly",
    "MicroShiftBoth",
    "StagerunOnly",
    "StagerunBoth",
    "ProdrunOnly",
    "ProdrunBoth",
    "CPaasrunOnly",
    "CPaasrunBoth",
    "Smokerun",
    "VMonly",
    "Longduration",
    "NonPreRelease",
    "PreChkUpgrade",
    "PstChkUpgrade",
    "DEPRECATED",
    "WRS"
]

# get the updated content
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

lines=[]
for filename in modifedFiles.decode("utf-8").strip(os.linesep).split():
    print("Search the updated cases for "+filename)
    diffcommands = 'git diff {} {} -- {} | grep -E "g.It|g.Describe|g.When|g.Context"'.format(commit1, commit2, filename.strip(os.linesep))
    # print(diffcommands)
    process = subprocess.Popen(diffcommands, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    try:
        outs, errs = process.communicate(timeout=300)
        if process.returncode == 0:
            output_lines = outs.decode('utf-8').strip().split('\n')
            for line in output_lines:
                # print(line)
                lines.append(line)
        else:
            print("Error occurred: {}".format(errs.decode('utf-8')))
    except subprocess.TimeoutExpired:
        process.kill()
        raise Exception(diffcommands +" timeout")
content = "\n".join(lines)
print("{}\n\n".format(content))


importance = ["Critical", "High", "Medium", "Low"]

patternDescribe = re.compile(r'^\+.*g\.Describe\("([^"]+)"', re.MULTILINE)
desContent = patternDescribe.findall(content)
patternContext = re.compile(r'^\+.*g\.Context\("([^"]+)"', re.MULTILINE)
ctxContent = patternContext.findall(content)
patternWhen = re.compile(r'^\+.*g\.When\("([^"]+)"', re.MULTILINE)
whenContent = patternWhen.findall(content)

patternIt = re.compile(r'^\+\s+(g\.It|g\.DescribeTable)\(\".*\"', re.MULTILINE)
itContent = []
itIters = patternIt.finditer(content)
for itIter in itIters:
    itContent.append(itIter.group(0))

displayDesc = "\n".join(desContent)
displayIt = "\n".join(itContent)
displayCtx = "\n".join(ctxContent)
displayWhen = "\n".join(whenContent)
print("Des:\n{} \n\n".format(displayDesc))
print("it:\n{} \n\n".format(displayIt))
print("Context:\n{} \n\n".format(displayCtx))
print("When:\n{} \n\n".format(displayWhen))

errList = []
for gctx in ctxContent:
    for frameworkLabel in frameworkLabels:
        if frameworkLabel in gctx:
            errList.append("g.Context \"{}\" has framework label \"{}\", and it should in \
framework labels parts of title, not in g.Context\n".format(gctx, frameworkLabel))
    for sub in subteam:
        if sub in gctx:
            errList.append("g.Context \"{}\" has subteam {}, please remove it because \
it should be in g.Describe even it is your own subteam because currently \
there is no way to check if it is your subteam or not. thanks to understand it \n".format(gctx, sub))


for gwhen in whenContent:
    for frameworkLabel in frameworkLabels:
        if frameworkLabel in gwhen:
            errList.append("g.When \"{}\" has framework label \"{}\", and it should in \
framework labels parts of title, not in g.When\n".format(gwhen, frameworkLabel))
    for sub in subteam:
        if sub in gwhen:
            errList.append("g.When \"{}\" has subteam {}, please remove it because \
it should be in g.Describe even it is your own subteam because currently \
there is no way to check if it is your subteam or not. thanks to understand it \n".format(gwhen, sub))

for des in desContent:
    sigSub = des.split()
    if len(sigSub) >= 2:
        sig = sigSub[0]
        sub = sigSub[1]
        # print(f"sig: {sig}, subTeam: {sub}")

        if not sig.strip("[]") in sigs:
            errList.append("g.Describe sig: {} in \"{}\" is not correct which is not in list\n".format(sig, des))

        if not sub in subteam:
            errList.append("g.Describe subteam: {} in \"{}\" is not correct which is not in list\n".format(sub, des))
    else:
        errList.append("the g.Describe \"{}\" is less than two words\n".format(des))

titlePatten = re.compile(r'\"(.*?)\"')
importancePatten = re.compile(r'(\w+)-([A-Z]?\d+)(-?)')
for it in itContent:
    # print(f"the it:\n{it}")
    it=it.replace("'", "")
    match = titlePatten.search(it)
    spec = patternIt.findall(it)[0]

    if match:
        title = match.group(1)

        if not title.startswith("Author:"):
            errList.append("{} \"{}\" does not start with \"Author:<your Kerberos ID>-\", please put it at the begining of the title\n".format(spec, title))

        for sub in subteam:
            if sub in title:
                errList.append("{} \"{}\" has subteam {}, please remove it because it should be in g.Describe even it is your own subteam because currently there is no way to check if it is your subteam or not. thanks to understand it \n".format(spec, title, sub))

        importances = importancePatten.finditer(title)        
        mList = list(importances)
        correctCaseID = True
        if len(mList) > 0:
            for m in mList:
                if not m.group(1) in importance:
                    errList.append("{} \"{}\" has wrong importance value {}\n".format(spec, title, m.group(1)))
                if len(m.group(2)) < 5:
                    errList.append("{} \"{}\" has wrong case id {}\n".format(spec, title, m.group(2)))
                    correctCaseID = False
                if not m.group(3):
                    errList.append("{} \"{}\" has no \"-\" after case id {}\n".format(spec, title, m.group(2)))
                    correctCaseID = False
            if correctCaseID:
                caseids = re.findall(r'\b[A-Z]?\d{5,}-', title)
                if len(caseids) == 0:
                    errList.append("{} \"{}\" has no case id\n".format(spec, title))
                else:
                    titleDescription = title.split(caseids[-1])[1]
                    if "[serial]" in titleDescription:
                        errList.append("case title \"{}\" has framework label \"[serial]\", and it should be \"[Serial]\"\n".format(titleDescription))
                    if "[disruptive]" in titleDescription:
                        errList.append("case title \"{}\" has framework label \"[disruptive]\", and it should be \"[Disruptive]\"\n".format(titleDescription))
                    for frameworkLabel in frameworkLabels:
                        if frameworkLabel in titleDescription:
                            errList.append("case title \"{}\" has framework label \"{}\", and it should in framework labels parts of title, not in title description\n".format(titleDescription, frameworkLabel))
        else:
            errList.append("{} \"{}\" has wrong importance format, please check it".format(spec, title))

    else:
        errList.append("the {} has no case title, please check the title\n".format(spec))

if len(errList) > 0:
    gKeyWord = "g.Describe or g.It"
    errList.append("""
Note:
We know it is new rule and the existing code does not follow it.
So, for existing code, you do not need dedicated PR to udpate {}.
Only when you modify the exiting {} for some other reasons or make new {}, please follow it.
""".format(gKeyWord, gKeyWord, gKeyWord))
    errs = "\n".join(errList)
    print("\nthe errors: \n{}\n".format(errs))
    exit(1)
