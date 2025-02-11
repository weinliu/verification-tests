#!/usr/bin/env python3
import argparse
from pkg_resources import invalid_marker
import requests
from requests.adapters import HTTPAdapter
from urllib3.util import Retry
import urllib3
from urllib3.exceptions import InsecureRequestWarning
import yaml
import re
import json
import os

class SummaryClient:
    SUBTEAM_OWNER = {
                "SDN":"@sdn-ovn-qe-team ",
                "STORAGE":"@storage-qe-team ",
                "Developer_Experience":"",
                "User_Interface":"",
                "PerfScale":"", 
                "Service_Development_B":"",
                "NODE":"",
                "Logging":"@logging-qe-team ",
                "Workloads":"@workloads-qe-team ",
                "Metering":"",
                "Cluster_Observability":"@monitoring-qe-team ",
                "Quay/Quay.io":"",
                "Cluster_Infrastructure":"",
                "Multi-Cluster":"",
                "Cluster_Operator":"",
                "Azure":"",
                "Network_Edge":"@ne-qe-team ",
                "ETCD":"@etcd-qe-team ",
                "Installer":"",
                "INSTALLER":"@jad",
                "Portfolio_Integration":"",
                "Service_Development_A":"",
                "Cluster_Management_Service":"",
                "OLM":"@olm-qe-team ",
                "Operator_SDK":"@operatorsdk-qe-team ",
                "App_Migration":"",
                "Windows_Containers":"",
                "Security_and_Compliance":"",
                "KNI":"",
                "Openshift_Jenkins":"",
                "RHV":"",
                "ISV_Operators":"@psap-qe-team ",
                "PSAP":"@psap-qe-team ",
                "Multi-Cluster-Networking":"",
                "OTA":"",
                "Kata":"",
                "Build_API":"",
                "Image_Registry":"@imageregistry-qe-team ",
                "Container_Engine_Tools":"",
                "MCO":"@rioliu ",
                "API_Server":"@apiserver-qe-team ",
                "Authentication":"@auth-qe-team ",
                "Hypershift":"",
                "Network_Observability":"@no-qe-team ",
                "DR_Testing":"@geliu",
                "OAP":"@oap-qe-team",
                "User_Interface_Cypress": "@ui-ocp-qe-team",
                "Insights": "@jfula",
                "Sample":"@Jitendar Singh"
            }
    def __init__(self, args):
        token = args.token
        if not token:
            if os.getenv('RP_TOKEN'):
                token = os.getenv('RP_TOKEN')
            else:
                if os.path.exists('/root/rp.key'):
                    with open('/root/rp.key', 'r') as outfile:
                        data = json.load(outfile)
                        token =data["ginkgo_rp_mmtoken"]
        if not token:
            raise BaseException("ERROR: token is empty, please input the token using -t")

        urllib3.disable_warnings(category=InsecureRequestWarning)
        self.session = requests.Session()
        self.session.headers["Authorization"] = "bearer {0}".format(token)
        self.session.verify = False
        retry = Retry(connect=3, backoff_factor=0.5)
        adapter = HTTPAdapter(max_retries=retry)
        self.session.mount('https://', adapter)
        self.session.mount('http://', adapter)

        self.base_url = "https://reportportal-openshift.apps.ocp-c1.prod.psi.redhat.com"
        self.launch_url = self.base_url +"/api/v1/ocp/launch"
        self.item_url = self.base_url + "/api/v1/ocp/item"
        self.ui_url = self.base_url + "/ui/#ocp/launches/all/"
        self.jenkins_url = "https://jenkins-csb-openshift-qe-mastern.dno.corp.redhat.com/job/ocp-common/job/Flexy-install/"
        self.slack_url = ""
        self.group_channel = args.group_channel
        if args.webhook_url:
            self.slack_url = args.webhook_url
        else:
            if self.group_channel and os.path.exists('/root/webhook_url_golang_ci_summary'):
                with open('/root/webhook_url_golang_ci_summary', 'r') as outfile:
                    data = json.load(outfile)
                    if self.group_channel in data.keys():
                        self.slack_url =data[self.group_channel]
        if not self.slack_url:
            print("WARNING: webhook_url is empty, will not send messsage to slack")

        self.launchnames = args.launchname
        self.subteam = args.subteam
        self.checkSubteam()
        self.releaseVersion = args.version
        self.cluster = args.cluster
        self.filterType = args.filter_type
        self.silence = args.silence
        self.additional_message = args.additional_message
        self.number = 0

    def checkSubteam(self):
        invalid_marker = False
        if self.subteam.lower() != "all":
            for s in self.subteam.split(":"):
                sr = s.strip()
                if sr == "isv]":
                    continue
                if sr not in self.SUBTEAM_OWNER.keys():
                    invalid_marker = True
                    print("subteam [{0}] is invalid, please double check the input value".format(sr))
        if invalid_marker:
            raise BaseException("ERROR: subteam name is invalid")
    def getLaunchIdWithLaunchName(self, launchname):
        launchs=dict()
        filter_url = self.launch_url + "?filter.eq.name={0}&filter.has.attributeValue=golang".format(launchname)
        if self.filterType == "equal":
            filter_url = self.launch_url + "?filter.eq.name={0}&filter.has.attributeValue=golang".format(launchname)
        elif self.filterType == "contain":
            filter_url = self.launch_url + "?filter.cnt.name={0}&filter.has.attributeValue=golang&page.size=2000".format(launchname)
        else:
            print("use default filter type: equal")

        #print(filter_url)
        try:
            r = self.session.get(url=filter_url)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("get launch error: {0}".format(r.text))
            ids = []
            #print(json.dumps(r.json(), indent=4, sort_keys=True))
            if len(r.json()["content"]) == 0:
                raise Exception("no launch found by name: {0}".format(launchname))
            for ret in r.json()["content"]:
                idOutput = ret["id"]
                launchs[idOutput] = dict()
                launchs[idOutput]["name"] = ret["name"]
                launchs[idOutput]["executions"] = dict()
                launchs[idOutput]["executions"]["total"] = ret["statistics"]["executions"]["total"]
                if "failed" in ret["statistics"]["executions"].keys():
                    launchs[idOutput]["executions"]["failed"] = ret["statistics"]["executions"]["failed"]
                else:
                    launchs[idOutput]["executions"]["failed"] = 0
                if "skipped" in ret["statistics"]["executions"].keys():
                    launchs[idOutput]["executions"]["skipped"] = ret["statistics"]["executions"]["skipped"]
                else:
                    launchs[idOutput]["executions"]["skipped"] = 0
                if "to_investigate" in ret["statistics"]["defects"].keys():
                    launchs[idOutput]["executions"]["to_investigate"] = ret["statistics"]["defects"]["to_investigate"]["total"]
                else:
                    launchs[idOutput]["executions"]["to_investigate"] = 0
                for attr in ret["attributes"]:
                    launchs[idOutput][attr["key"]] = attr["value"]
                if "profilename" not in launchs[idOutput].keys():
                    launchs.pop(idOutput, None)
                elif self.releaseVersion:
                    if launchs[idOutput]["version"] != self.releaseVersion:
                        launchs.pop(idOutput, None)
            if not launchs:
                raise Exception("ERROR: no Launch is found".format(ids))
            #print(launchs)
            return launchs
        except BaseException as e:
            print(e)
            return launchs

    def getFailCaseID(self, launchId):
        item_url = self.item_url + "?filter.eq.launchId={0}&filter.eq.status=FAILED&isLatest=false&launchesLimit=0&page.size=150".format(launchId)
        # print(item_url)
        try:
            r = self.session.get(url=item_url)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("get item case error: {0}".format(r.text))
            caseidList = []
            if len(r.json()["content"]) == 0:
                return "No fail case"
            #print(json.dumps(r.json(), indent=4, sort_keys=True))
            for ret in r.json()["content"]:
                if ret["type"] == "STEP":
                    caseids = re.findall(r'OCP-\d{4,}', ret["name"])
                    if len(caseids) > 0:
                        caseAuthor = ret["name"].split(":")[1]
                        caseidList.append(caseids[0][4:]+"-"+caseAuthor)
            if len(caseidList) == 0:
                return ""
            separator = '|'
            return separator.join(caseidList)
        except BaseException as e:
            print(e)

    def notifyToSlack(self, notificationList=[]):
        try:
            msgList = []
            for notification in notificationList:
                msgList.append({"type": "section","text": {"type":"mrkdwn","text": notification}})
            msg = {"blocks": msgList}
            r = self.session.post(url=self.slack_url, json=msg)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("send slack message error: {0}".format(r.text))
            return r.status_code 
        except BaseException as e:
            print(e)
            print("\n")
            return None

    def collectResult(self, launchname):
        result = dict()
        launchs = self.getLaunchIdWithLaunchName(launchname)
        for launchID in launchs.keys():
            teamName = launchs[launchID]["team"]
            if teamName in self.subteam.split(":") or self.subteam.lower() == "all":
                testrunName = launchs[launchID]['name']
                if testrunName not in result.keys():
                    result[testrunName] = dict()
                    result[testrunName]["profilename"] = launchs[launchID]["profilename"]
                    result[testrunName]["build_version"] = launchs[launchID]["build_version"]
                    result[testrunName]["gbuildnum"] = launchs[launchID]["gbuildnum"]
                
                if "caseResult" not in result[testrunName].keys():
                    result[testrunName]["caseResult"]=dict()
                result[testrunName]["caseResult"][teamName] = dict()
                result[testrunName]["caseResult"][teamName]["launchID"] = launchID
                result[testrunName]["caseResult"][teamName]["faildCase"] = self.getFailCaseID(launchID)
                result[testrunName]["caseResult"][teamName]["total"] = launchs[launchID]["executions"]["total"]
                result[testrunName]["caseResult"][teamName]["failed"] = launchs[launchID]["executions"]["failed"]
                result[testrunName]["caseResult"][teamName]["skipped"] = launchs[launchID]["executions"]["skipped"]
                result[testrunName]["caseResult"][teamName]["to_investigate"] = launchs[launchID]["executions"]["to_investigate"]
        return result
    
    def collectResultToSlack(self, launchname):
        result = self.collectResult(launchname)
        for testrun in result.keys():
            notificationList = []
            notificationHeader=[]
            notificationHeader.append("****************************************************************")
            notificationHeader.append("******      golang test result:"+testrun+"                      ******")
            notificationHeader.append("profile:"+result[testrun]["profilename"])
            notificationHeader.append("build_version:"+result[testrun]["build_version"])
            notificationHeader.append("gbuildnum:"+result[testrun]["gbuildnum"])
            notificationList.append("\n".join(notificationHeader))
            faildTeamOwner =""
            for subteam in result[testrun]["caseResult"].keys():
                notificationSub=[]
                failedNumber = result[testrun]["caseResult"][subteam]["failed"]
                if failedNumber == 0:
                   continue 
                total=result[testrun]["caseResult"][subteam]["total"]
                skipped =result[testrun]["caseResult"][subteam]["skipped"]
                toInvestigateNumber = result[testrun]["caseResult"][subteam]["to_investigate"]
                link = self.ui_url +str(result[testrun]["caseResult"][subteam]["launchID"])
                notificationSub.append("---------- subteam: "+subteam+" -------------")
                notificationSub.append("total: {0}, failed: {1}, skipped: {2}, to_investigate: {3}, {4} ".format(total, failedNumber, skipped, toInvestigateNumber, link))
                notificationSub.append("Failed Cases: "+result[testrun]["caseResult"][subteam]["faildCase"])
                if "No fail case" not in result[testrun]["caseResult"][subteam]["faildCase"]:
                    if subteam in self.SUBTEAM_OWNER.keys():
                        faildTeamOwner = faildTeamOwner + self.SUBTEAM_OWNER[subteam]
                notificationList.append("\n".join(notificationSub))
            notificationEnd = []
            self.number = self.number+1
            if not self.silence:
                debugMsg = "{0} Please debug failed cases, thanks!".format(faildTeamOwner)
                if self.cluster:
                    debugMsg = debugMsg + " Cluster:{0}{1}".format(self.jenkins_url, self.cluster)
                notificationEnd.append(debugMsg)
            if self.additional_message:
                notificationEnd.append(self.additional_message)
            notificationEnd.append("\n")
            notificationList.append("\n".join(notificationEnd))
            print("\n".join(notificationList))
            if self.slack_url:
                self.notifyToSlack(notificationList)

    def collectAllResultToSlack(self):
        for launchname in self.launchnames.split(":"):
            print("collect result of "+launchname)
            self.collectResultToSlack(launchname)

if __name__ == "__main__":
    parser = argparse.ArgumentParser(prog="python3 collect_result_to_slack.py", usage='''%(prog)s -l <launchname> -s <subteam> -t <token>''')
    parser.add_argument("-t","--token", default="")
    parser.add_argument("-s","--subteam", default="", required=True, help="subteam in g.Describe, separator is colon, eg OLM:OperatorSDK")
    parser.add_argument("-l","--launchname", default="", required=True, help="the lauchname which is the value of LAUNCH_NAME of the job, separator is colon")
    parser.add_argument("-v","--version", default="", help="the release version, eg:4_10")
    parser.add_argument("-c","--cluster", default="", help="the jenkins build number of the cluster for debugging")
    parser.add_argument("-f","--filter_type", default="equal", help="the search type, only support equal/contain")
    parser.add_argument("-w","--webhook_url", default="", help="the webhook url used to send message")
    parser.add_argument("-g","--group_channel", default="", help="the channel name which will be send result to")
    parser.add_argument("-a","--additional_message", default="", help="additional message")
    parser.add_argument("--silence", dest='silence', default=False, action='store_true', help="the flag to request debug")
    args=parser.parse_args()

    sclient = SummaryClient(args)
    sclient.collectAllResultToSlack()
    
    exit(0)

