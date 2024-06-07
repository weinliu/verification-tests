#!/usr/bin/env python3
import xml.dom.minidom
import argparse
import re
import codecs
import os

class TestResult:
    subteam = [
                "SDN","STORAGE","Developer_Experience","User_Interface","PerfScale", "Service_Development_B","NODE","LOGGING","Logging",
                "Workloads","Metering","Cluster_Observability","Quay/Quay.io","Cluster_Infrastructure",
                "Multi-Cluster","Cluster_Operator","Azure","Network_Edge","ETCD","INSTALLER","Portfolio_Integration",
                "Service_Development_A","OLM","Operator_SDK","App_Migration","Windows_Containers","Security_and_Compliance",
                "KNI","Openshift_Jenkins","RHV","ISV_Operators","PSAP","Multi-Cluster-Networking","OTA","Kata","Build_API",
                "Image_Registry","Container_Engine_Tools","MCO","API_Server","Authentication","Hypershift","Network_Observability",
                "DR_Testing","CFE","User_Interface_Cypress","Insights","Sample", "Cluster_Management_Service"
            ]

    frameworkLabels = [
        "DisconnectedOnly-",
        "ConnectedOnly-",
        "OSD_NONCCS-",
        "OSD_CCS-",
        "ARO-",
        "ROSA-",
        "HyperShiftMGMT-",
        "NonHyperShiftHOST-",
        "MicroShiftOnly-",
        "MicroShiftBoth-",
        "StagerunOnly-",
        "StagerunBoth-",
        "ProdrunOnly-",
        "ProdrunBoth-",
        "CPaasrunOnly-",
        "CPaasrunBoth-",
        "Smokerun-",
        "VMonly-",
        "Longduration-",
        "NonPreRelease-",
        "PreChkUpgrade-",
        "PstChkUpgrade-",
        "DEPRECATED-",
        "Critical-",
        "High-",
        "Medium-",
        "Low-",
        "LEVEL0-"
    ]

    coSubteamMap = {
            "authentication": "Authentication",
            "baremetal": "INSTALLER",
            "cloud-controller-manager": "Cluster_Infrastructure",
            "cloud-credential": "Cluster_Operator",
            "cluster-autoscaler": "Cluster_Infrastructure",
            "config-operator": "API_Server",
            "console": "User_Interface_Cypress",
            "control-plane-machine-set": "Cluster_Infrastructure",
            "csi-snapshot-controller": "STORAGE",
            "dns": "Network_Edge",
            "etcd": "ETCD",
            "image-registry": "Image_Registry",
            "ingress": "Network_Edge",
            "insights": "Insights",
            "kube-apiserver": "API_Server",
            "kube-controller-manager": "Workloads",
            "kube-scheduler": "Workloads",
            "kube-storage-version-migrator": "API_Server",
            "machine-api": "Cluster_Infrastructure",
            "machine-approver": "Cluster_Infrastructure",
            "machine-config": "MCO",
            "marketplace": "OLM",
            "monitoring": "Cluster_Observability",
            "network": "SDN",
            "node-tuning": "PSAP",
            "openshift-apiserver": "API_Server",
            "openshift-controller-manager": "Workloads",
            "openshift-samples": "Sample",
            "operator-lifecycle-manager": "OLM",
            "operator-lifecycle-manager-catalog": "OLM",
            "operator-lifecycle-manager-packageserver": "OLM",
            "service-ca": "API_Server",
            "storage": "STORAGE",
            "cluster-api": "Cluster_Infrastructure",
            "olm": "OLM",
            "platform-operators-aggregated": "OLM",
    }

    def __init__(self):
        self.isJenkinsEnv = "JENKINS_AGENT_NAME" in os.environ
        print("isJenkinsEnv: {0}\\n".format(str(self.isJenkinsEnv)))
        # it checks if the script is executed in jenkins to support potential sippy integration
        # if it is executed in jenkins, it means the result is not in gcs and keep current logic
        # if it is executed in others (prow), it means the result is in gcs and possible modify the
        # the following to support sippy integration
        # A: testsuite name is not subteam name. and change to certain nanem
        #    for prow-test-results-classfier, when it uses testsuite name, it should get it from file name,
        #    not junit xml. and when it imports result to reportportal, it should update testsuite name got
        #    from file name.
        #B: junit file name is possible changed to "junit-import-*xml" from "import-*xml"
        #    for prow-test-results-classfier, need to support new file format
        #    for prow golang step, need to support new file format
        # NOW it only handle testsuite name firstly.

    def removeMonitor(self, input, output):
        noderoot = xml.dom.minidom.parse(input)

        testsuites = noderoot.getElementsByTagName("testsuite")

        cases = noderoot.getElementsByTagName("testcase")
        toBeRemove = None
        totalnum = 0
        failnum = 0
        skipnum = 0
        for case in cases:
            value = case.getAttribute("name")
            if "Monitor cluster while tests execute" in value:
                toBeRemove = case
                continue
            totalnum = totalnum + 1
            if len(case.getElementsByTagName("failure")) != 0:
                failnum = failnum + 1
            if len(case.getElementsByTagName("skipped")) != 0:
                skipnum = skipnum + 1
        if toBeRemove is not None:
            noderoot.firstChild.removeChild(toBeRemove)

        testsuites[0].setAttribute("tests", str(int(totalnum)))
        testsuites[0].setAttribute("failures", str(int(failnum)))
        testsuites[0].setAttribute("skipped", str(int(skipnum)))
        # it is not used by golang framework, and here hard-coded it as 0 to compatible with other tools
        testsuites[0].setAttribute("errors", "0")
        with open(output, 'wb+') as f:
            writer = codecs.lookup('utf-8')[3](f)
            noderoot.writexml(writer, encoding='utf-8')
            writer.close()

    def pirntResult(self, input):
        testsummary = {}
        result = ""
        noderoot = xml.dom.minidom.parse(input)
        cases = noderoot.getElementsByTagName("testcase")
        for case in cases:
            name = case.getAttribute("name")
            if "Monitor cluster while tests execute" in name:
                continue
            failure = case.getElementsByTagName("failure")
            skipped = case.getElementsByTagName("skipped")
            result = "PASS"
            if skipped:
                result="SKIP"
            if failure:
                result="FAIL"
            caseids = re.findall(r'\d{5,}-', name)
            authorname = self.getAuthorName(name)
            if len(caseids) == 0:
                tmpname = name.replace("'","")
                if "[Suite:openshift/" in tmpname:
                    testsummary["No-CASEID Author:"+authorname+" "+tmpname.split("[Suite:openshift/")[-2]] = {"result":result, "title":"", "author":""}
                else:
                    testsummary["No-CASEID Author:"+authorname+" "+tmpname] = {"result":result, "title":"", "author":""}
            else:
                casetitle = name.split(caseids[-1])[1]
                if "[Suite:openshift/" in casetitle:
                    casetitle = casetitle.split("[Suite:openshift/")[0]

                casetitle = self.combineTilteAndDescribe(casetitle, authorname, name, caseids)

                for i in caseids:
                    id = "OCP-"+i[:-1]
                    if id in testsummary:
                        if "FAIL" in testsummary[id]["result"]: #the case already execute with failure
                            result = testsummary[id]["result"]
                            casetitle = testsummary[id]["title"]
                        if ("PASS" in testsummary[id]["result"]) and (result == "SKIP"):
                            result = testsummary[id]["result"]
                            casetitle = testsummary[id]["title"]
                    testsummary[id] = {"result":result, "title":casetitle.replace("'",""), "author":authorname}
        print("The Case Execution Summary:\\n")
        output = ""
        for k in sorted(testsummary.keys()):
            output += " "+testsummary[k]["result"]+"  "+k.replace("'","")+"  Author:"+testsummary[k]["author"]+"  "+testsummary[k]["title"]+"\\n"
        print(output)

    def generateRP(self, input, output, scenario):
        noderoot = xml.dom.minidom.parse(input)
        testsuites = noderoot.getElementsByTagName("testsuite")
        if self.isJenkinsEnv:
            newsuitename = scenario
        else:
            newsuitename = scenario # it will change to other after we get testsuite name rule for sippy.
        testsuites[0].setAttribute("name", newsuitename)

        cases = noderoot.getElementsByTagName("testcase")
        toBeRemove = []
        toBeAdd = []
        #do not support multiple case implementation for one OCP case if we take only CASE ID as name.
        for case in cases:
            name = case.getAttribute("name")
            caseids = re.findall(r'\d{5,}-', name)
            authorname = self.getAuthorName(name)
            if len(caseids) == 0:
                # print("No Case ID")
                tmpname = name.replace("'","")
                if "[Suite:openshift/" in tmpname:
                    case.setAttribute("name", "No-CASEID:"+authorname+":" + scenario + ":" + tmpname.split("[Suite:openshift/")[-2])
                else:
                    case.setAttribute("name", "No-CASEID:"+authorname+":" + scenario + ":" + tmpname)
            else:
                # print("Case ID exists")
                casetitle = name.split(caseids[-1])[1].replace("'","")
                if "[Suite:openshift/" in casetitle:
                    casetitle = casetitle.split("[Suite:openshift/")[0]

                casetitle = self.combineTilteAndDescribe(casetitle, authorname, name, caseids)

                if len(caseids) == 1:
                    case.setAttribute("name", "OCP-"+caseids[0][:-1]+":"+authorname+":" + scenario + ":" +casetitle)
                else:
                    toBeRemove.append(case)
                    for i in caseids:
                        casename = "OCP-"+i[:-1]+":"+authorname+":" + scenario + ":" +casetitle
                        dupcase = case.cloneNode(True)
                        dupcase.setAttribute("name", casename)
                        toBeAdd.append(dupcase)
        # print(toBeRemove)
        # print(toBeAdd)
        #ReportPortal does not depeond on failures and tests to count the case number, so no need to update them
        for case in toBeAdd:
            noderoot.firstChild.appendChild(case)
        for case in toBeRemove:
            noderoot.firstChild.removeChild(case)

        with open(output, 'w+') as f:
            writer = codecs.lookup('utf-8')[3](f)
            noderoot.writexml(writer, encoding='utf-8')
            writer.close()

    def splitRP(self, input):
        noderoot = xml.dom.minidom.parse(input)
        origintestsuite = noderoot.getElementsByTagName("testsuite")[0]
        mods = {}

        cases = noderoot.getElementsByTagName("testcase")
        for case in cases:
            failcount = 0
            skippedcount = 0
            errorcount = 0
            if len(case.getElementsByTagName("failure")) != 0:
                failcount = 1
            if len(case.getElementsByTagName("skipped")) != 0:
                skippedcount = 1
            name = case.getAttribute("name")
            subteam = name.split(" ")[1]
            if not subteam in self.subteam:
                subteam = "Unknown"
            # print(subteam)
            names = self.getNames(name, subteam)
            # print(names)
            casedesc = {"case": case, "names":names}
            mod = mods.get(subteam)
            # adjust to that tests is the total case, skipped is skipped case, and failures is only failure case.
            if mod is not None:
                mod["cases"].append(casedesc)
                mod["tests"] = mod["tests"] + 1
                mod["skipped"] = mod["skipped"] + skippedcount
                mod["failure"] = mod["failure"] + failcount
                # mod["tests"] = mod["tests"] + 1 - skippedcount
                # mod["skipped"] = mod["skipped"] + skippedcount
                # mod["failure"] = mod["failure"] + failcount + skippedcount
            else:
                mods[subteam] = {"cases": [casedesc], "tests": 1, "skipped": skippedcount, "failure": failcount, "errors": errorcount}
                # mods[subteam] = {"cases": [casedesc], "tests": 1 - skippedcount, "skipped": skippedcount, "failure": failcount + skippedcount}

        for k, v in mods.items():
            impl = xml.dom.minidom.getDOMImplementation()
            newdoc = impl.createDocument(None, None, None)
            testsuite = newdoc.createElement("testsuite")
            testscount = v["tests"]
            failurescount = v["failure"]
            skippedcount = v["skipped"]
            errorcount = v["errors"]
            testsuite.setAttribute("time", origintestsuite.getAttribute("time")) #RP does not depend on it
            if self.isJenkinsEnv:
                newsuitename = k
            else:
                newsuitename = k # it will change to other after we get testsuite name rule for sippy.
            testsuite.setAttribute("name", newsuitename)

            for case in v["cases"]:
                testnum = 0
                failnum = 0
                skipnum = 0
                result = "PASS"
                if len(case["case"].getElementsByTagName("skipped")) != 0:
                    result = "SKIP"
                if len(case["case"].getElementsByTagName("failure")) != 0:
                    result = "FAIL"

                for name in case["names"]:
                    if result == "PASS":
                        testnum = testnum + 1
                    if result == "FAIL":
                        testnum = testnum + 1
                        failnum = failnum + 1
                    if result == "SKIP":
                        skipnum = skipnum + 1
                        testnum = testnum + 1
                        # failnum = failnum + 1
                    dupcase = case["case"].cloneNode(True)
                    dupcase.setAttribute("name", name)
                    testsuite.appendChild(dupcase)

                if testnum > 0:
                    testnum = testnum -1
                if failnum > 0:
                    failnum = failnum -1
                if skipnum > 0:
                    skipnum = skipnum -1
                testscount = testscount + testnum
                failurescount = failurescount + failnum
                skippedcount = skippedcount + skipnum

            testsuite.setAttribute("tests", str(testscount))
            testsuite.setAttribute("failures", str(failurescount))
            testsuite.setAttribute("skipped", str(skippedcount))
            testsuite.setAttribute("errors", str(errorcount))
            newdoc.appendChild(testsuite)

            with open("import-"+k+".xml", 'wb+') as f:
                writer = codecs.lookup('utf-8')[3](f)
                newdoc.writexml(writer, encoding='utf-8')
                writer.close()


    def combineTilteAndDescribe(self, casetitle, author, name, caseids):
        tmpTitle = casetitle
        if author == "unknown":
            tmpTitle = "NOAUTHOR please correct case " + tmpTitle
        else:
            casepre = name.replace("'","").split(caseids[-1])[0].split("Author:")[0]
            for label in self.frameworkLabels:
                labelWithoutMinus = label.rstrip("-")
                casepre = casepre.replace(label, "").replace(labelWithoutMinus, "")
            tmpTitle = casepre + tmpTitle
        return tmpTitle

    def getNames(self, name, subteam):
        names = []
        caseids = re.findall(r'\d{5,}-', name)
        authorname = self.getAuthorName(name)
        if len(caseids) == 0:
            # print("No Case ID")
            tmpname = name.replace("'","")
            if "[Suite:openshift/" in tmpname:
                names.append("No-CASEID:"+authorname+":" + subteam + ":" + tmpname.split("[Suite:openshift/")[-2])
            else:
                names.append("No-CASEID:"+authorname+":" + subteam + ":" + tmpname)
        else:
            # print("Case ID exists")
            casetitle = name.split(caseids[-1])[1].replace("'","")
            if "[Suite:openshift/" in casetitle:
                casetitle = casetitle.split("[Suite:openshift/")[0]

            casetitle = self.combineTilteAndDescribe(casetitle, authorname, name, caseids)

            for i in caseids:
                names.append("OCP-"+i[:-1]+":"+authorname+":" + subteam + ":"+casetitle)
        return names

    def getAuthorName(self, name):
        authors = "unknown"
        authorfilter = re.findall(r'Author:\w+-', name)
        if len(authorfilter) != 0:
            authors = authorfilter[0][:-1].split(":")[1]
        # print(authors)
        return authors

    def getSubteamByCO(self, co):
        subteam = self.coSubteamMap.get(co)
        # print(co)
        if subteam is None:
            return "NoCO"
        return subteam

    def healcheckRP(self, input, steptype, clusteroperator, scenario, recognize):

        impl = xml.dom.minidom.getDOMImplementation()
        newdoc = impl.createDocument(None, None, None)
        testsuite = newdoc.createElement("testsuite")

        testsuite.setAttribute("errors", "0")
        testsuite.setAttribute("failures", "1")
        if self.isJenkinsEnv:
            newsuitename = scenario
        else:
            newsuitename = scenario # it will change to other after we get testsuite name rule for sippy.
        testsuite.setAttribute("name", newsuitename)
        testsuite.setAttribute("skipped", "0")
        testsuite.setAttribute("tests", "1")
        testsuite.setAttribute("time", "1")

        testcase = newdoc.createElement("testcase")
        testcase.setAttribute("name", "OCP-00000:{0}_leader:clusteroperator {1} fails at {2} healthcheck".format(scenario, clusteroperator, steptype))
        if recognize == "no":
            testcase.setAttribute("name", "OCP-00000:Kui:{0}:clusteroperator {1} fails at {2} healthcheck, but no owner, please query with team to add it".format(scenario, clusteroperator, steptype))
        testcase.setAttribute("time", "1")

        failure = newdoc.createElement("failure")
        # failure.setAttribute("message", "clusteroperator {0} is still in progressing.\n please take link in the description of the launch to check it".format(clusteroperator))
        failure.setAttribute("message", "")
        failure_text = newdoc.createTextNode("clusteroperator {0} is still in progressing.\n please take link in the description of the launch to check it".format(clusteroperator))
        failure.appendChild(failure_text)

        testcase.appendChild(failure)
        testsuite.appendChild(testcase)

        if input != "":
            noderoot = xml.dom.minidom.parse(input)
            origtestsuite = noderoot.getElementsByTagName("testsuite")[0]
            tests = origtestsuite.getAttribute("tests")
            failures = origtestsuite.getAttribute("failures")
            testsuite.setAttribute("failures", str(int(failures)+1))
            testsuite.setAttribute("tests", str(int(tests)+1))

            cases = noderoot.getElementsByTagName("testcase")
            for case in cases:
                testsuite.appendChild(case)

        newdoc.appendChild(testsuite)
        with open("import-"+scenario+"bak.xml", 'wb+') as f:
            writer = codecs.lookup('utf-8')[3](f)
            newdoc.writexml(writer, encoding='utf-8')
            writer.close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser("handleresult.py")
    parser.add_argument("-a","--action", default="get", choices={"replace", "get", "generate", "split", "comap", "hcj"}, required=True)
    parser.add_argument("-i","--input", default="")
    parser.add_argument("-o","--output", default="")
    parser.add_argument("-s","--scenario", default="")
    parser.add_argument("-co","--clusteroperator", default="")
    parser.add_argument("-r","--recognize", default="")
    parser.add_argument("-st","--steptype", default="", choices={"e2e", "preupg", "pstupg"})
    args=parser.parse_args()

    testresult = TestResult()
    if args.action == "comap":
        print(testresult.getSubteamByCO(args.clusteroperator))
        exit(0)

    if args.action == "hcj":
        testresult.healcheckRP(args.input, args.steptype, args.clusteroperator, args.scenario, args.recognize)
        exit(0)

    if args.input == "":
        print("please provide input paramter")
        exit(1)
    if args.action == "get":
        testresult.pirntResult(args.input)
        exit(0)
    if args.action == "split":
        testresult.splitRP(args.input)
        exit(0)

    if args.output == "":
        print("please provide output paramter")
        exit(1)
    if args.action == "replace":
        testresult.removeMonitor(args.input, args.output)

    if args.action == "generate":
        testresult.generateRP(args.input, args.output, args.scenario)
        exit(0)
