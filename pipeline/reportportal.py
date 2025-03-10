#!/usr/bin/env python3
import argparse
import requests
from requests.adapters import HTTPAdapter
from urllib3.util import Retry
import urllib3
from urllib3.exceptions import InsecureRequestWarning
from datetime import datetime, timedelta
import os
import yaml
import xml.dom.minidom
import re
import urllib.parse

class ReportPortalClient:
    subteam = [
                "SDN","STORAGE","Developer_Experience","User_Interface","PerfScale", "Service_Development_B","NODE","LOGGING","Logging",
                "Workloads","Metering","Cluster_Observability","Quay/Quay.io","Cluster_Infrastructure",
                "Multi-Cluster","Cluster_Operator","Azure","Network_Edge","ETCD","INSTALLER","Portfolio_Integration",
                "Service_Development_A","OLM","Operator_SDK","App_Migration","Windows_Containers","Security_and_Compliance",
                "KNI","Openshift_Jenkins","RHV","ISV_Operators","PSAP","Multi-Cluster-Networking","OTA","Kata","Build_API",
                "Image_Registry","Container_Engine_Tools","MCO","API_Server","Authentication","Hypershift","Network_Observability",
                "DR_Testing","OAP","User_Interface_Cypress","Insights","Sample", "Cluster_Management_Service"
            ]
    def __init__(self, args):
        urllib3.disable_warnings(category=InsecureRequestWarning)
        self.session = requests.Session()
        self.session.headers["Authorization"] = "bearer {0}".format(args.token)
        self.session.verify = False
        retry = Retry(connect=3, backoff_factor=0.5)
        adapter = HTTPAdapter(max_retries=retry)
        self.session.mount('https://', adapter)
        self.session.mount('http://', adapter)
        # os.environ['no_proxy'] = "reportportal-openshift.apps.dno.ocp-hub.prod.psi.redhat.com"
        self.session.trust_env = False

        self.launch_url = args.endpoint + "/v1/" + args.project + "/launch"
        self.item_url = args.endpoint + "/v1/" + args.project + "/item"
        self.log_url = args.endpoint + "/v1/" + args.project + "/log"
        self.args = args
        # print (self.session.headers)

    def logResult(self):
        try:
            launchname = os.path.splitext(os.path.basename(self.args.file))[0]
            existinglaunch = self.getLaunchIdWithLaunchName(launchname, {"key": "team", "value":self.args.subteam})
            print(existinglaunch)
            print("\\n")
            # return True
            if existinglaunch == None:
                return self.importResult()
            else:
                # if self.deleteLaunchById(existinglaunch[0]):
                #     return self.importResult()
                # else:
                #     raise Exception('can not delete exiting launch, so can not import rerun result')
                return self.rerunResult(launchname, existinglaunch)
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def rerunResult(self, launchname, existinglaunch):
        suiteuuid = None
        containeruuid = None
        launchrestarted = False
        suiteduration = 0
        suiteresult = "PASSED"
        existingid = existinglaunch[0]
        existinguuid = existinglaunch[1]
        try:
            starttime = existinglaunch[2]
            # finishtime = existinglaunch[3]
            # starttime = datetime.fromtimestamp(timestamp/1000.0)
            starttime = datetime.utcfromtimestamp(starttime/1000.0 + 1)
            # starttime = datetime.utcnow()
            # timediff = datetime.strptime(datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%S.%f'), "%Y-%m-%dT%H:%M:%S.%f").strftime('%s.%f')
            # starttime = int(float(timediff)*1000)
            # print(starttime)

            noderoot = xml.dom.minidom.parse("import-"+self.args.subteam+".xml")
            testsuites = noderoot.getElementsByTagName("testsuite")
            cases = noderoot.getElementsByTagName("testcase")
            suitname = testsuites[0].getAttribute("name")
            # print(suitname)

            deletedList = []
            shouldDeletedList = []
            existingCaseTimeMap = {}
            for case in cases:
                casename = case.getAttribute("name")
                childitem = self.getChild(existingid, casename)
                if childitem is not None:
                    existingCaseTimeMap[casename] = {"startTime": childitem[0]["startTime"], "endTime": childitem[0]["endTime"]}
                    for child in childitem:
                        if self.deleteChild(child["id"]):
                            deletedList.append(casename)
                        else:
                            shouldDeletedList.append(casename)

            self.addMoreBuildNumToLaunch(existingid)

            launchrestarted = self.startLaunch(launchname, existinguuid, starttime)
            if not launchrestarted:
                raise Exception('rerun start launch fails')

            suiteuuid = self.startSuite(suitname, existinguuid, starttime) #it should be startTest, but keep this name for less code change.
            # print(suiteuuid)
            if suiteuuid == None:
                raise Exception('start suite fails')
            containeruuid = suiteuuid

            # the orginal V5 return same id for SUITE and TEST following launch->suite->test->step
            # but now it return different id for SUITE and TEST. and when importing result, it follows launch->test->step
            # so, we change to startSuite from STUIE to TEST, and disable startContainer
            # containeruuid = self.startContainer(suiteuuid, suitname, existinguuid, starttime)
            # print(containeruuid)
            # if containeruuid == None:
            #     raise Exception('start container fails')
            
            for case in cases:
                casename = case.getAttribute("name")
                casetime = int(float(case.getAttribute("time")))
                failureinfos = case.getElementsByTagName("failure")
                skippedinfos = case.getElementsByTagName("skipped")
                systemoutinfos = case.getElementsByTagName("system-out")

                casestarttime = starttime + timedelta(0,suiteduration)
                caseendtime = casestarttime + timedelta(0,casetime)
                suiteduration = suiteduration + casetime
                if len(failureinfos) != 0:
                    suiteresult = "FAILED"
                
                if (casename in deletedList) and (not casename in shouldDeletedList):
                    childitemid = self.replaceChild(containeruuid, casename, existinguuid, existingCaseTimeMap[casename]["startTime"])
                    if childitemid is not None:
                        self.finishReplaceChild(childitemid, existinguuid, existingCaseTimeMap[casename]["startTime"], existingCaseTimeMap[casename]["endTime"], failureinfos, skippedinfos, systemoutinfos)

                if (not casename in deletedList) and (not casename in shouldDeletedList):
                    childitemid = self.createChild(containeruuid, casename, existinguuid, casestarttime)
                    if childitemid is not None:
                        self.finishCreateChild(childitemid, existinguuid, casestarttime, caseendtime, failureinfos, skippedinfos, systemoutinfos)

            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False
        finally:
            # if containeruuid is not None:
            #     self.finishContainer(containeruuid, existinguuid, starttime, suiteduration)
            if suiteuuid is not None:
                self.finishSuite(suiteuuid, existinguuid, starttime, suiteduration)
            if launchrestarted:
                self.finishLaunch(suiteresult, existinguuid, starttime, suiteduration)

    def deleteChild(self, childid):
        try:
            r = self.session.delete(url=self.item_url+"/"+str(childid))
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("delete child error: {0}".format(r.text))
            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def getChild(self, launchId, childname):
        try:
            r = self.session.get(url=self.item_url+"?filter.eq.launchId="+str(launchId)+"&filter.eq.name="+childname+"&isLatest=false&launchesLimit=0")
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("get child error: {0}".format(r.text))
            childs = r.json()["content"]
            if len(childs) == 0:
                raise Exception('no child')
            return childs
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def replaceChild(self, parentid, childname, existinguuid, currenttime):
        currenttime = datetime.utcfromtimestamp(currenttime/1000.0 + 1)
        return self.startChild(parentid, childname, existinguuid, currenttime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z")
    def finishReplaceChild(self, childid, existinguuid, starttime, finishtime, failures, skipped, systemouts):
        return self.finishChild(childid, existinguuid, str(starttime), str(finishtime), failures, skipped, systemouts)
    def createChild(self, parentid, childname, existinguuid, currenttime):
        return self.startChild(parentid, childname, existinguuid, currenttime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z")
    def finishCreateChild(self, childid, existinguuid, starttime, finishtime, failures, skipped, systemouts):
        return self.finishChild(childid, existinguuid, starttime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z", finishtime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z", failures, skipped, systemouts)


    def startChild(self, parentid, childname, existinguuid, currenttime):
        try:
            itemdata = {
                "name": childname,
                "startTime": currenttime,
                # "startTime": currenttime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z",
                "type": "STEP",
                "launchUuid": existinguuid
            }
            r = self.session.post(url=self.item_url+"/"+parentid, json=itemdata)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("start child error: {0}".format(r.text))
            return r.json()["id"]
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def finishChild(self, childid, existinguuid, starttime, finishtime, failures, skipped, systemouts):
        try:
            # finishtime = starttime + timedelta(0,casetime)
            if len(failures) != 0 or len(skipped) != 0: #not be failure and skipped at same time
                childstatus = "FAILED"
                if len(skipped) != 0:
                    childstatus = "SKIPPED"
                itemdata = {
                    "endTime": finishtime,
                    "launchUuid": existinguuid,
                    "status": childstatus,
                    "issue": {
                        "issueType": "ti001",
                        "autoAnalyzed": "false",
                        "ignoreAnalyzer": "false",
                        "externalSystemIssues": []
                    }
                }
                failuredata = None
                if len(failures) != 0:
                    failuredata = {
                        "launchUuid": existinguuid,
                        "itemUuid": childid,
                        "time": starttime,
                        "message": failures[0].firstChild.nodeValue,
                        "level": "ERROR"
                    }
                systemoutdata = {
                    "launchUuid": existinguuid,
                    "itemUuid": childid,
                    "time": starttime,
                    "message": systemouts[0].firstChild.nodeValue,
                    "level": "INFO"
                }
                if failuredata is not None:
                    r = self.session.post(url=self.log_url, json=failuredata)
                    # print(r.status_code)
                    # print(r.text)
                    if (r.status_code != 200) and (r.status_code != 201):
                        raise Exception("save error log into child error: {0}".format(r.text))
                r = self.session.post(url=self.log_url, json=systemoutdata)
                # print(r.status_code)
                # print(r.text)
                if (r.status_code != 200) and (r.status_code != 201):
                    raise Exception("save systemout log into child error: {0}".format(r.text))
            else:
                itemdata = {
                    # "endTime": finishtime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z",
                    "endTime": finishtime,
                    "launchUuid": existinguuid,
                    "status": "PASSED"
                }
            r = self.session.put(url=self.item_url+"/"+childid, json=itemdata)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("finish child error: {0}".format(r.text))
            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def startContainer(self, parentid, containername, existinguuid, currenttime):
        try:
            # print(currenttime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z")
            itemdata = {
                "launchUuid": existinguuid,
                "name": containername,
                # "startTime": str(currenttime),
                "startTime": currenttime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z",
                "type": "TEST"
            }
            r = self.session.post(url=self.item_url+"/"+parentid, json=itemdata)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("start container error: {0}".format(r.text))
            return r.json()["id"]
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def finishContainer(self, containerid, existinguuid, starttime, suiteduration):
        try:
            finishtime = starttime + timedelta(0,suiteduration)
            # print(finishtime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z")
            # finishtime = starttime + int(float(suiteduration)*1000)
            itemdata = {
                "endTime": finishtime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z",
                # "endTime": str(finishtime),
                "launchUuid": existinguuid
            }
            r = self.session.put(url=self.item_url+"/"+containerid, json=itemdata)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("finish root item error: {0}".format(r.text))
            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def startSuite(self, suitename, existinguuid, currenttime):
        try:
            # print(currenttime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z")
            itemdata = {
                "launchUuid": existinguuid,
                "name": suitename,
                # "startTime": str(currenttime),
                "startTime": currenttime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z",
                "type": "TEST" # change it from SUITE to TEST to apply same with importing result.
            }
            r = self.session.post(url=self.item_url, json=itemdata)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("start suite error: {0}".format(r.text))
            return r.json()["id"]
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def finishSuite(self, itemid, existinguuid, starttime, suiteduration):
        try:
            finishtime = starttime + timedelta(0,suiteduration)
            # print(finishtime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z")
            # finishtime = starttime + int(float(suiteduration)*1000)
            itemdata = {
                # "endTime": str(finishtime),
                "endTime": finishtime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z",
                "launchUuid": existinguuid
            }
            r = self.session.put(url=self.item_url+"/"+itemid, json=itemdata)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("finish suite error: {0}".format(r.text))
            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def startLaunch(self, launchname, existinguuid, currenttime):
        try:
            # print(currenttime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z")
            startlaunchdata = {
                "mode": "DEFAULT",
                "name": launchname,
                "rerun": "true",
                "rerunOf": existinguuid,
                # "startTime": str(currenttime)
                "startTime": currenttime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z"
            }
            r = self.session.post(url=self.launch_url, json=startlaunchdata)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("rerun start launch error: {0}".format(r.text))
            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def finishLaunch(self, suiteresult, existinguuid, starttime, suiteduration):
        try:
            finishtime = starttime + timedelta(0,suiteduration)
            # print(finishtime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z")
            # finishtime = starttime + int(float(suiteduration)*1000)
            finishlaunchdata = {
                "endTime": finishtime.strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z",
                "status": suiteresult
            }
            r = self.session.put(url=self.launch_url+"/"+existinguuid+"/finish", json=finishlaunchdata)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("rerun finish launch error: {0}".format(r.text))
            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def importResult(self):
        import_url = self.launch_url + "/import"
        attrKeyList = ["plannedin","caseautomation","env_network_plugin","env_container_runtime","env_auth","env_iaas_cloud_provider","env_os","env_docker_storage_driver",
            "env_install_method","env_network_backend","env_cluster","env_fips","env_disconnected","env_behind_proxy","env_private_cluster","env_networking_address",
            "products"
            ]
        # print(import_url)
        try:
            files = {'file': (self.args.file, open(self.args.file,'rb'), 'application/zip')}
            r = self.session.post(url=import_url, files=files, timeout=120)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("import error: {0}".format(r.text))
            # if (r.status_code != 200 and r.status_code != 504):
            #     raise Exception("import error: {0}".format(r.text))
            # if (r.status_code == 504 and ("server didn't respond in time" not in r.text)):
            #     raise Exception("import error: {0}".format(r.text))
            # if (r.status_code == 504 and ("server didn't respond in time" in r.text)):
            #     pass

            id = self.getLaunchIdWithLaunchUuid(r.json()["message"].split("id =")[1].strip().split(" ")[0])
            if id == None:
                raise Exception("can not get id for new launch")
            attMap = self.getProfileAttr()

            buildVersion = self.getAttrOption("build_version")
            if buildVersion == None or buildVersion == "":
                buildVersion = "nobuildversion"
            pipelineType = self.getAttrOption("pipeline_type")
            if pipelineType == None or pipelineType == "":
                pipelineType = "notpipeline"
            telcoProfileName = self.getAttrOption("telco_profile_name")
            if telcoProfileName == None or telcoProfileName == "":
                telcoProfileName = "unknowntelcoprofilename"

            if attMap == None:
                attDict = {
                "name":     {"action": "add", "value":os.path.splitext(os.path.basename(self.args.file))[0]},
                "team":     {"action": "add", "value":self.args.subteam},
                "version":  {"action": "add", "value":self.args.version.replace(".", "_")},
                "build_version":  {"action": "add", "value":buildVersion},
                "pipeline_type":  {"action": "add", "value":pipelineType},
                "gbuildnum": {"action": "add", "value":self.args.buildnum},
                "launchtype": {"action": "add", "value":"golang"},
                }
                if telcoProfileName != "unknowntelcoprofilename":
                    attDict["profilename"] = {"action": "add", "value":telcoProfileName}
            else:
                finalProfileName = self.args.profilename
                if telcoProfileName != "unknowntelcoprofilename":
                    finalProfileName = telcoProfileName
                attDict = {
                "name":     {"action": "add", "value":os.path.splitext(os.path.basename(self.args.file))[0]},
                "team":     {"action": "add", "value":self.args.subteam},
                "version":  {"action": "add", "value":self.args.version.replace(".", "_")},
                "build_version":  {"action": "add", "value":buildVersion},
                "pipeline_type":  {"action": "add", "value":pipelineType},
                "gbuildnum": {"action": "add", "value":self.args.buildnum},
                "launchtype": {"action": "add", "value":"golang"},
                "profilename": {"action": "add", "value":finalProfileName},
                }
                if self.args.triallaunch == "yes":
                    attDict["trial"] = {"action": "add", "value":"\"\""}
                else:
                    attDict["nontrial"] = {"action": "add", "value":"\"\""}
                for attrKey in attrKeyList:
                    if attMap["custom_fields"].__contains__(attrKey) is True:
                        if attrKey == "products":
                            productName = "\"\""
                            if attMap["custom_fields"][attrKey][0] != "":
                                productName = attMap["custom_fields"][attrKey][0]
                            attDict[attrKey] = {"action": "add", "value":productName}
                        else:
                            attrValue = "\"\""
                            if attMap["custom_fields"][attrKey] != "":
                                attrValue = attMap["custom_fields"][attrKey]
                            attDict[attrKey] = {"action": "add", "value":attrValue}
                # "env_private_cluster":          {"action": "add", "value":attMap["custom_fields"]["env_private_cluster"]},
                # "env_networking_address":       {"action": "add", "value":attMap["custom_fields"]["env_networking_address"]},
                # "products":                     {"action": "add", "value":attMap["custom_fields"]["products"][0]},
                # }
            if not self.handleLaunchAttribution([id], attDict):
                raise Exception("fail to add attrs")

            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def mergeResult(self):
        ids = self.getLaunchIdWithLaunchName(self.args.launchname)
        if ids == None:
            return False
        merge_url = self.launch_url + "/merge"
        # print(merge_url)
        currenttime = datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%S.%f')[:-3] + "Z"
        # could add more attributes to add logical for launch
        data = {
                "attributes": [
                    {
                    "key": "combined",
                    "value": "yes"
                    }
                ],
                "description": "testrun " + self.args.launchname,
                "endTime": currenttime,
                "extendSuitesDescription": "true",
                "launches": ids,
                "mergeType": "BASIC",
                "mode": "DEFAULT",
                "name": self.args.launchname,
                "startTime": currenttime
                }
        try:
            r = self.session.post(url=merge_url, json=data)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("merge error: {0}".format(r.text))
            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def deleteLaunchById(self, launchid):
        data = {
                "ids": [launchid]
            }
        try:
            r = self.session.delete(url=self.launch_url, json=data)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("delete launch by ID error: {0}".format(r.text))
            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def deleteLaunchByName(self):
        ids = self.getLaunchIdWithLaunchName(self.args.launchname)
        if ids == None:
            return False
        data = {
                "ids": ids
                }
        try:
            r = self.session.delete(url=self.launch_url, json=data)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("delete launch error: {0}".format(r.text))
            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def checkLaunchsByNameScenario(self, launchname, scenarios):
        filter_url = self.launch_url + "?filter.eq.name=" + launchname
        # print(filter_url)
        try:
            r = self.session.get(url=filter_url)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("get launch error: {0}".format(r.text))
            retContent = r.json()["content"]

            if len(retContent) == 0:
                return {"LAUNCHRESULT": ["NEWLAUNCH"], "SUBMATCHRESULT": "NONE"}

            hasGolangLaunch = False
            for ret in retContent:
                if self.isGolangLaunch(ret["attributes"]):
                    hasGolangLaunch = True
            if not hasGolangLaunch:
                return {"LAUNCHRESULT": ["NEWGOLANGLAUNCH"], "SUBMATCHRESULT": "NONE"}

            subTeamList = self.parseScenarios(scenarios)
            if len(subTeamList) == 0:
                return {"LAUNCHRESULT": ["NOSUBTEAMINSCENARIO"], "SUBMATCHRESULT": "NONE"}

            ids = ["EXISTINGLAUNCH"]
            subteamNotMatched = []
            for st in subTeamList:
                matched = False
                for ret in retContent:
                    if not self.isGolangLaunch(ret["attributes"]):
                        continue

                    buildnum = ""
                    for attr in ret["attributes"]:
                        if attr["key"] == "gbuildnum" and attr.get("value") != None:
                            buildnum = attr["value"]

                    for attr in ret["attributes"]:
                        if attr["key"] == "team" and attr["value"] == st and buildnum != "":
                            for bid in buildnum.split(","):
                                if re.match('(\d+)-([a-zA-Z]+)_(.+)', bid) is None:
                                    continue
                                bidType = bid.replace(" ", "").split("_")[0].split("-")[1]
                                bidJobName = "ginkgo-test" # old build num format, take default value
                                if len(bid.replace(" ", "").split("_")) > 1:
                                    bidJobName = bid.replace(" ", "").split("_")[1] # new build num format
                                if (bidJobName == self.args.buildnum.split("_")[1] and
                                    bidType == self.args.buildnum.split("_")[0].split("-")[1]):
                                    # launch, team, build job_name, and  buildtype already exist
                                    ids.append(ret["id"])
                                    matched = True
                if not matched:
                    subteamNotMatched.append(st)

            return {"LAUNCHRESULT": ids, "SUBMATCHRESULT": subteamNotMatched}
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def getFailCaseID(self):
        checkresult = self.checkLaunchsByNameScenario(self.args.launchname, self.args.scenarios)
        if checkresult is None:
            return "\\nNOFOUND-GETLAUNCHERROR-NOREPLACE"

        if checkresult["LAUNCHRESULT"][0] == "NEWLAUNCH":
            return "\\nNOFOUND-NEWLAUNCH-NOREPLACE"

        if checkresult["LAUNCHRESULT"][0] == "NEWGOLANGLAUNCH":
            return "\\nNOFOUND-NEWGOLANGLAUNCH-NOREPLACE"

        if checkresult["LAUNCHRESULT"][0] == "NOSUBTEAMINSCENARIO":
            return "\\nNOFOUND-NOSUBTEAMINSCENARIO-NOREPLACE"

        notMatchedSubTeamInScenario = ""
        if len(checkresult["SUBMATCHRESULT"]) != 0:
            notMatchedSubTeamInScenario = '|'.join(checkresult["SUBMATCHRESULT"])
        nonSubTeamInScenario = self.getNonSubTeamPerScenarios(self.args.scenarios)
        notFailCase = ""
        if notMatchedSubTeamInScenario != "" and nonSubTeamInScenario != "":
            notFailCase = notMatchedSubTeamInScenario + "|" + nonSubTeamInScenario
        elif notMatchedSubTeamInScenario == "":
            notFailCase = nonSubTeamInScenario
        else:
            notFailCase = notMatchedSubTeamInScenario

        launchId = checkresult["LAUNCHRESULT"][1:]
        returnString = ""
        try:
            failCaseList = []
            getFailReason = {}
            for lid in launchId:
                # print(lid)
                item_url = self.item_url + "?filter.eq.launchId={0}&filter.eq.status=FAILED&isLatest=false&launchesLimit=0&page.size=300".format(lid)
                #suppose one launch instance will have max 300 failed case. if not, will handle it with page.number.
                # print(item_url)
                r = self.session.get(url=item_url)
                # print(r.status_code)
                # print(r.text)
                if (r.status_code != 200):
                    getFailReason[lid] = "get launch id {0} item error: {1}".format(lid, r.text)
                    continue

                if len(r.json()["content"]) == 0:
                    # no fail case for this launch instance
                    # print("no fail case")
                    continue

                for ret in r.json()["content"]:
                    # print(ret["type"])
                    # print(ret["name"])
                    if ret["type"] == "STEP":
                        caseids = re.findall(r'OCP-\d{4,}', ret["name"])
                        if len(caseids) > 0:
                            failCaseList.append(caseids[0][4:])
                # print(failCaseList)

            for _, v in getFailReason.items():
                returnString = returnString + "\\n"+v
            if len(failCaseList) == 0 and notFailCase == "":
                return returnString + "\\nNOFAILEDCASEFOUNDNONEWCASE-NORERUN"

            if returnString != "":
                returnString = returnString + "\\nThere is error to get failcase althoug we already get some fails case. maybe need to rerun again"

            returnString = returnString + "\\n"

            finalReplaceScenario = ""
            if len(failCaseList) != 0:
                finalReplaceScenario = finalReplaceScenario + "|".join(failCaseList)
            if notFailCase != "":
                if finalReplaceScenario == "":
                    finalReplaceScenario = notFailCase
                else:
                    finalReplaceScenario = finalReplaceScenario + "|" + notFailCase

            finalReplaceScenario = finalReplaceScenario.replace("ISV_Operators", "isv]")
            return returnString + finalReplaceScenario
        except BaseException as e:
            print(e)
            return returnString + "\\nEXCEPTION-NOREPLACE"

    def isGolangLaunch(self, attrs):
        isGolang = False
        for attr in attrs:
            if attr["key"] == "launchtype" and attr["value"] == "golang":
            # in the future, maybe add launchtype=cucu and then add check attr["value"] != "cucu"
                isGolang = True
        return isGolang

    def getLaunchIdWithLaunchName(self, launchname, attrfilter=None):
        filter_url = self.launch_url + "?page.page=1&page.size=300&filter.eq.name=" + launchname
        # print(filter_url)
        try:
            r = self.session.get(url=filter_url)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("get ID error: {0}".format(r.text))
            ids = []
            for ret in r.json()["content"]:
                if not self.isGolangLaunch(ret["attributes"]):
                    continue

                if attrfilter == None:
                    ids.append(ret["id"])
                else:
                    for attr in ret["attributes"]:
                        if attr["key"] == attrfilter["key"] and attr["value"] == attrfilter["value"]:
                            ids.append(ret["id"])
                            ids.append(ret["uuid"])
                            ids.append(ret["startTime"])
                            ids.append(ret["endTime"])
            # print(ids)
            if len(ids) == 0:
                raise Exception('no id return')
            return ids
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def getLaunchIdWithLaunchUuid(self, uuid):
        uuid_url = self.launch_url + "/uuid/" + uuid
        try:
            r = self.session.get(url=uuid_url)
            if (r.status_code != 200):
                raise Exception("get ID with uuid error: {0}".format(r.text))
            return r.json()["id"]
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def handleLaunchAttribution(self, idList, attrDict):
        update_url = self.launch_url + "/info"
        attrList = []
        for k, v in attrDict.items():
            if v["action"] == "update":
                oldkv = {"key":k, "value":v["oldvalue"]}
                newkv = {"key":k, "value":v["newvalue"]}
                att = {"action": "UPDATE", "from": oldkv, "to": newkv}
            else:
                kv = {"key":k, "value":v["value"]}
                if v["action"] == "add":
                    att = {"action": "CREATE", "to": kv}
                else:
                    att = {"action": "DELETE", "from": kv}
            attrList.append(att)
        data = {"attributes": attrList, "ids": idList}
        try:
            r = self.session.put(url=update_url, json=data, headers={"Authorization": "bearer {0}".format(args.tatoken)})
            if (r.status_code != 200):
                raise Exception("update attr error: {0}".format(r.text))
            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def addMoreBuildNumToLaunch(self, lid):
        try:
            existingattrvalue = self.getLaunchAttrByID(lid, "gbuildnum")
            if existingattrvalue == None or existingattrvalue == "":
                raise Exception("fail to get attr or no such attr")

            if self.args.buildnum in existingattrvalue:
                #build id already exists
                return True

            buildType = self.args.buildnum.split("_")[0].split("-")[1]
            buildJobName = self.args.buildnum.split("_")[1]

            for bid in existingattrvalue.split(","):
                if re.match('(\d+)-([a-zA-Z]+)_(.+)', bid) is None:
                    continue
                bidType = bid.replace(" ", "").split("_")[0].split("-")[1]
                bidJobName = "ginkgo-test" # old build num format, take default value
                if len(bid.replace(" ", "").split("_")) > 1:
                    bidJobName = bid.replace(" ", "").split("_")[1] # new build num format
                if bidType == buildType and bidJobName == buildJobName:
                    #same build type and build job name already exist
                    return True

            newattrvalue = existingattrvalue + "," + self.args.buildnum
            attDict = {
                "gbuildnum": {"action": "update", "oldvalue":existingattrvalue, "newvalue":newattrvalue},
                }

            if not self.handleLaunchAttribution([lid], attDict):
                raise Exception("fail to add more buildnum")

            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False

    def getLaunchAttrByID(self, lid, attrkey):
        lid_url = self.launch_url + "/" + str(lid)
        try:
            r = self.session.get(url=lid_url)
            if (r.status_code != 200):
                raise Exception("get attr of launch id {0} error: {1}".format(lid, r.text))
            retAttr = r.json()["attributes"]
            attrvalue = ""
            for attr in retAttr:
                if attr["key"] == attrkey:
                    attrvalue = attr["value"]
            return attrvalue
        except BaseException as e:
            print(e)
            print("\\n")
            return None


    def getProfileAttr(self):
        filename = self.args.profilepath + self.args.profilename + ".test_run.yaml"
        # print(filename)
        try:
            with open(filename) as f:
                attrmap = yaml.safe_load(f)
            # print(attrmap["custom_fields"]["products"][0])
            return attrmap
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def getAttrOption(self, name):
        if self.args.attroption == "":
            return None
        try:
            attrvalue = yaml.safe_load(self.args.attroption)
            namepath = name.split(":")
            for i in namepath:
                attrvalue = attrvalue[i]
            return attrvalue
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def parseScenarios(self, scenarios):
        valideSubTeam = []
        scenarioList = scenarios.split("|")
        for s in scenarioList:
            sr = s.strip()
            if sr == "isv]":
                valideSubTeam.append("ISV_Operators")
            if sr in self.subteam:
                valideSubTeam.append(sr)
        return valideSubTeam

    def getNonSubTeamPerScenarios(self, scenarios):
        nonSubTeam = []
        scenarioList = scenarios.split("|")
        for s in scenarioList:
            sr = s.strip()
            if sr != "isv]" and (sr not in self.subteam):
                nonSubTeam.append(sr)

        if len(nonSubTeam) == 0:
            return ""
        return "|".join(nonSubTeam)

    def existingGolangLaunchWithProfile(self, profilename, version):
        attribute_url = self.launch_url + "?page.page=1&page.size=10&filter.has.attributeValue=" + urllib.parse.quote(profilename+","+version+",golang")
        try:
            r = self.session.get(url=attribute_url, timeout=180)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("get attribute of profile {0} error: {1}".format(profilename, r.text))
            launches = r.json()["content"]
            if len(launches) == 0:
                return None
            return len(launches)
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def getFakeGolangLaunchProfileList(self, launchname, version):
        fake_launch_url = self.launch_url + "?page.page=1&page.size=300&filter.has.attributeValue=" + urllib.parse.quote(version+",golang")+"&filter.eq.name="+launchname
        try:
            r = self.session.get(url=fake_launch_url, timeout=180)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("get fake launch with name {0} error: {1}".format(launchname, r.text))
            profileList = []
            for ret in r.json()["content"]:
                for attr in ret["attributes"]:
                    if attr["key"] == "profilename":
                        profileList.append(attr["value"])
            profileList = list(set(profileList))
            # print(len(profileList))
            return profileList
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def putProfileLaunch(self):
        pathToProfile = self.args.profilepath

        #get current profile list
        profileList = []
        for f in os.listdir(pathToProfile):
            # if f.endswith(".test_run.yaml") and (not f.endswith("destructive.test_run.yaml")) and os.path.isfile(os.path.join(pathToProfile, f)) and (not f.startswith("tmp")):
            if f.endswith(".test_run.yaml") and (not f.endswith("destructive.test_run.yaml")) and os.path.isfile(os.path.join(pathToProfile, f)):
                profileList.append(os.path.splitext(os.path.splitext(f)[0])[0])
        # print(profileList)
        # print(len(profileList))
        if len(profileList) == 0:
            print("NOTFOUNDPROFILELIST and no need to create fake profile")
            return

        #get fake launch profile list to delete launch if it does not exist in real profile
        launchname = "00000000-00"+self.args.version.replace(".", "")
        existingFakeLaunchProfileList = self.getFakeGolangLaunchProfileList(launchname, self.args.version.replace(".", "_"))
        if existingFakeLaunchProfileList is not None:
            for existingProfile in existingFakeLaunchProfileList:
                if not (existingProfile in profileList):
                    print("\n-----{0}--{1}-----".format(existingProfile, self.args.version.replace(".", "_")))
                    print("profile is removed in repo and so delete fake launch")
                    existinglaunch = self.getLaunchIdWithLaunchName(launchname, {"key": "profilename", "value":existingProfile})
                    print(existinglaunch)
                    if existinglaunch is not None:
                        self.deleteLaunchById(existinglaunch[0])

        for profile in profileList:
            print("\n#####{0}##{1}#####".format(profile, self.args.version.replace(".", "_")))
            existingGolangLaunch = self.existingGolangLaunchWithProfile(profile, self.args.version.replace(".", "_"))
            if existingGolangLaunch is not None:
                print("HAS launch, so no need to create fake launch")
                if existingGolangLaunch > 1:
                    print("    more than one launch")
                    existinglaunch = self.getLaunchIdWithLaunchName(launchname, {"key": "profilename", "value":profile})
                    print(existinglaunch)
                    if existinglaunch is None:
                        print("       no fake launch and so no neede to delete it.")
                    if existinglaunch is not None:
                        print("       delete fake launch")
                        self.deleteLaunchById(existinglaunch[0])
            else:
                print("NO launch, so create fake launch")
                self.args.profilename = profile
                self.importResult()


if __name__ == "__main__":
    parser = argparse.ArgumentParser("reportportal.py")
    parser.add_argument("-a","--action", default="import", choices={"import", "merge", "get", "delete", "attr", "getprofile", "getfcd", "putprofile"}, required=True)
    parser.add_argument("-e","--endpoint", default="https://reportportal-openshift.apps.dno.ocp-hub.prod.psi.redhat.com/api")
    parser.add_argument("-t","--token", default="")
    parser.add_argument("-ta","--tatoken", default="")
    parser.add_argument("-p","--project", default="ocp")
    #import, getprofile
    parser.add_argument("-f","--file", default="")
    parser.add_argument("-s","--subteam", default="")
    parser.add_argument("-v","--version", default="")
    parser.add_argument("-ao","--attroption", default="")
    parser.add_argument("-pn","--profilename", default="09_Disconnected UPI on Azure with RHCOS & Private Cluster")
    parser.add_argument("-pp","--profilepath", default="../cip/")
    #merge, getwithlanuchname, delete, getfcd
    parser.add_argument("-l","--launchname", default="")
    parser.add_argument("-ss","--scenarios", default="notnull")
    #handle attr
    parser.add_argument("-id","--launchid", default="")
    parser.add_argument("-aa","--attract", default="")
    parser.add_argument("-key","--attrkey", default="")
    parser.add_argument("-value","--attrvalue", default="")
    parser.add_argument("-trial","--triallaunch", default="yes")
    parser.add_argument("-bn","--buildnum", default="unknown")
    args=parser.parse_args()

    rpc = ReportPortalClient(args)
    if args.action == "import":
        if rpc.logResult():
            print("SUCCESS")
        else:
            print("FAIL")

    if args.action == "merge":
        if rpc.mergeResult():
            print("SUCCESS")
        else:
            print("FAIL")

    if args.action == "delete":
        if rpc.deleteLaunchByName():
            print("SUCCESS")
        else:
            print("FAIL")

    if args.action == "get":
        print(rpc.getLaunchIdWithLaunchName(args.launchname))
    if args.action == "getprofile":
        rpc.getProfileAttr()
    if args.action == "getfcd":
        print(rpc.getFailCaseID())
    if args.action == "attr":
        attDict = {args.attrkey: {"action":args.attract, "value":args.attrvalue}}
        if rpc.handleLaunchAttribution([args.launchid], attDict):
            print("SUCCESS")
        else:
            print("FAIL")
    if args.action == "putprofile":
        rpc.putProfileLaunch()
    exit(0)

