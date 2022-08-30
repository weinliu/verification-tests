#!/usr/bin/env python3
import xml.dom.minidom
import argparse
import re
import codecs

class TestResult:
    subteam = [
                "SDN","STORAGE","Developer_Experience","User_Interface","PerfScale", "Service_Development_B","NODE","LOGGING","Logging",
                "Workloads","Metering","Cluster_Observability","Quay/Quay.io","Cluster_Infrastructure",
                "Multi-Cluster","Cluster_Operator","Azure","Network_Edge","ETCD","Installer","Portfolio_Integration",
                "Service_Development_A","OLM","Operator_SDK","App_Migration","Windows_Containers","Security_and_Compliance",
                "KNI","Openshift_Jenkins","RHV","ISV_Operators","PSAP","Multi-Cluster-Networking","OTA","Kata","Build_API",
                "Image_Registry","Container_Engine_Tools","MCO","API_Server","Authentication","Hypershift","Network_Observability",
                "DR_Testing"
            ]

    def removeMonitor(self, input, output):
        noderoot = xml.dom.minidom.parse(input)

        testsuites = noderoot.getElementsByTagName("testsuite")
        
        testedcasenum = testsuites[0].getAttribute("failures")
        #here is the issue for xml. failures is the total executed cases, not include skip
        failcasenum = testsuites[0].getAttribute("tests")
        #here is the issue for xml. tests is the total failing and skip cases
        #skip is the skip cases
        #the expected is that tests is executed case, and skip is skipped cases, and failures is failing and skipped cases
        #the total case is skipped + tests
        
        cases = noderoot.getElementsByTagName("testcase")
        toBeRemove = None
        for case in cases:
            value = case.getAttribute("name")
            if "Monitor cluster while tests execute" in value:
                testsuites[0].setAttribute("tests", str(int(testedcasenum)-1))
                testsuites[0].setAttribute("failures", str(int(failcasenum)-1))
                toBeRemove = case
                break
        if toBeRemove is not None:
            noderoot.firstChild.removeChild(toBeRemove)

        # Before this change, tests is the executed case number, skipped is  the skipped case number, so the total should be
        # tests+skipped. failures is the failures case and skipped cases number.
        # for example, tests=23, skipped=2, failures=5, it means total case is 25, and the real failure case is 3
        # Per https://help.catchsoftware.com/display/ET/JUnit+Format, I try it change to the following:
        # tests is the total case, skipped is skipped case, and failures is only failure case.
        # For the above example, tests=25, skipped=2, failures=3
        totalnum = int(testsuites[0].getAttribute("tests")) + int(testsuites[0].getAttribute("skipped"))
        failnum = int(testsuites[0].getAttribute("failures")) - int(testsuites[0].getAttribute("skipped"))
        testsuites[0].setAttribute("tests", str(int(totalnum)))
        testsuites[0].setAttribute("failures", str(int(failnum)))
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
        testsuites[0].setAttribute("name", scenario)

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
                    case.setAttribute("name", "No-CASEID:"+authorname+":" + tmpname.split("[Suite:openshift/")[-2])
                else:
                    case.setAttribute("name", "No-CASEID:"+authorname+":" + tmpname)
            else:
                # print("Case ID exists")
                casetitle = name.split(caseids[-1])[1].replace("'","")
                if "[Suite:openshift/" in casetitle:
                    casetitle = casetitle.split("[Suite:openshift/")[0]
                if len(caseids) == 1:
                    case.setAttribute("name", "OCP-"+caseids[0][:-1]+":"+authorname+":"+casetitle)
                else:
                    toBeRemove.append(case)
                    for i in caseids:
                        casename = "OCP-"+i[:-1]+":"+authorname+":"+casetitle
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
            names = self.getNames(name)
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
            testsuite.setAttribute("name", k)

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


    def getNames(self, name):
        names = []
        caseids = re.findall(r'\d{5,}-', name)
        authorname = self.getAuthorName(name)
        if len(caseids) == 0:
            # print("No Case ID")
            tmpname = name.replace("'","")
            if "[Suite:openshift/" in tmpname:
                names.append("No-CASEID:"+authorname+":" + tmpname.split("[Suite:openshift/")[-2])
            else:
                names.append("No-CASEID:"+authorname+":" + tmpname)
        else:
            # print("Case ID exists")
            casetitle = name.split(caseids[-1])[1].replace("'","")
            if "[Suite:openshift/" in casetitle:
                casetitle = casetitle.split("[Suite:openshift/")[0]
            for i in caseids:
                names.append("OCP-"+i[:-1]+":"+authorname+":"+casetitle)
        return names

    def getAuthorName(self, name):
        authors = "unknown"
        authorfilter = re.findall(r'Author:\w+-', name)
        if len(authorfilter) != 0:
            authors = authorfilter[0][:-1].split(":")[1]
        # print(authors)
        return authors

if __name__ == "__main__":
    parser = argparse.ArgumentParser("handleresult.py")
    parser.add_argument("-a","--action", default="get", choices={"replace", "get", "generate", "split"}, required=True)
    parser.add_argument("-i","--input", default="", required=True)
    parser.add_argument("-o","--output", default="")
    parser.add_argument("-s","--scenario", default="")
    args=parser.parse_args()

    
    testresult = TestResult()
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
