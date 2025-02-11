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
                "SDN":"@sdn-ovn-qe-team",
                "STORAGE":"@storage-qe-team",
                "Developer_Experience":"",
                "User_Interface":"@ui-qe-team",
                "PerfScale":"@perfscale-qe-team",
                "Service_Development_B":"@xueli",
                "NODE":"@node-qe-team",
                "Logging":"@logging-qe-team",
                "Workloads":"@workloads-qe-team",
                "Metering":"@pruan",
                "Cluster_Observability":"@monitoring-qe-team",
                "Quay/Quay.io":"@quay-qe-team",
                "Cluster_Infrastructure":"@cloud-qe-team",
                "Multi-Cluster":"",
                "Cluster_Operator":"@hive-qe-team",
                "Azure":"",
                "Network_Edge":"@ne-qe-team",
                "ETCD":"@etcd-qe-team",
                "Installer":"@installer-qe-team",
                "INSTALLER":"@jad",
                "Portfolio_Integration":"",
                "Service_Development_A":"@yasun",
                "Cluster_Management_Service":"@yasun",
                "OLM":"@olm-qe-team ",
                "Operator_SDK":"@operatorsdk-qe-team ",
                "App_Migration":"@xjiang",
                "Windows_Containers":"@winc-qe-team",
                "Security_and_Compliance":"@isc-qe-team",
                "KNI":"",
                "Openshift_Jenkins":"",
                "RHV":"",
                "ISV_Operators":"@psap-qe-team",
                "PSAP":"@psap-qe-team ",
                "Multi-Cluster-Networking":"",
                "OTA":"@ota-qe-team",
                "Kata":"@kata-qe-team",
                "Build_API":"@jitensingh",
                "Image_Registry":"@imageregistry-qe-team",
                "Container_Engine_Tools":"@pmali",
                "MCO":"@mco-qe-team",
                "API_Server":"@apiserver-qe-team",
                "Authentication":"@auth-qe-team",
                "Hypershift":"@hypershift-qe-team",
                "Network_Observability":"@no-qe-team",
                "DR_Testing":"@geliu",
                "OAP":"@oap-qe-team",
                "User_Interface_Cypress": "@ui-qe-team",
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
                        token =data["ginkgo_rp_prow_token"]
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
        self.launch_url = self.base_url +"/api/v1/prow/launch"
        self.item_url = self.base_url + "/api/v1/prow/item"
        self.ui_url = self.base_url + "/ui/#prow/launches/all/"
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

        self.launchID = args.launchID
        self.subteam = args.subteam
        if not self.subteam:
            self.subteam = "all"
        self.checkSubteam()
        self.releaseVersion = args.version
        self.cluster = args.cluster
        self.silence = args.silence
        self.upgrade = args.upgrade
        self.ignore_investigated = args.ignore_investigated
        self.additional_message = args.additional_message
        self.number = 0
        self.days = 1
        if args.days:
            self.days = int(args.days)


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
    def getLaunchIdWithLaunchName(self, launchID):
        launchs=dict()
        filterVersion = "version:"+self.releaseVersion
        if self.upgrade:
            filterVersion = "to:"+self.releaseVersion

        filter_url = self.launch_url + '?&filter.has.compositeAttribute={0}&filter.btw.startTime=-{1};1440;-0000&page.size=2000'.format(filterVersion, str(1440*self.days))
        if self.launchID:
            filter_url = self.launch_url + "/"+self.launchID
        #print("filter_url is "+filter_url)
        try:
            r = self.session.get(url=filter_url)
            if (r.status_code != 200):
                if (r.status_code == 403):
                    error_msg = "Got response code 403, which indicate an invalid URL ({0}), please make sure the run id is correct". format(filter_url, launchID)
                else:
                    error_msg = "get launch error: {0}".format(r.text)
                raise Exception(error_msg)
            ids = []
            #print(json.dumps(r.json(), indent=4, sort_keys=True))
            if not self.launchID:
                if len(r.json()["content"]) == 0:
                    raise Exception("no launch found by name: {0}".format(launchID))
                for ret in r.json()["content"]:
                    idOutput = ret["id"]
                    print("check testrun: "+str(idOutput))
                    launchs[idOutput] = dict()
                    launchs[idOutput]["name"] = ret["name"]
                    launchs[idOutput]["description"] = ret["description"]
                    launchs[idOutput]["number"] = ret["number"]
            else:
                ret = r.json()
                idOutput = ret["id"]
                launchs[idOutput] = dict()
                launchs[idOutput]["name"] = ret["name"]
                launchs[idOutput]["description"] = ret["description"]
                launchs[idOutput]["number"] = ret["number"]
            if not launchs:
                raise Exception("ERROR: no Launch is found".format(ids))
            #print(launchs)
            return launchs
        except BaseException as e:
            print(e)
            return launchs

    def getFailCaseID(self, launchId):
        item_url = self.item_url + "?filter.eq.launchId={0}&filter.eq.status=FAILED&isLatest=false&launchesLimit=0&page.size=150".format(launchId)
        #print(item_url)
        try:
            r = self.session.get(url=item_url)
            if (r.status_code != 200):
                raise Exception("get item case error: {0}".format(r.text))
            caseidList = []
            FailedCase = dict()
            if len(r.json()["content"]) == 0:
                return FailedCase
            #print(json.dumps(r.json(), indent=4, sort_keys=True))
            FailedCase = dict()
            for ret in r.json()["content"]:
                if ret["type"] == "STEP":
                    subteamOut = ret["pathNames"]["itemPaths"][0]["name"]
                    defectsType = ret["statistics"]["defects"].keys()
                    if self.ignore_investigated and "to_investigate" not in defectsType:
                        continue
                    else:
                        if subteamOut not in FailedCase.keys():
                            FailedCase[subteamOut] = []
                        caseids = re.findall(r'OCP-\d{4,}', ret["name"])
                        if len(caseids) > 0:
                            if "cucushift" in subteamOut:
                                caseAuthor = ""
                                FailedCase[subteamOut].append(caseids[0][4:])
                            else:
                                if ":" in ret["name"]:
                                    caseAuthor = ret["name"].split(":")[1]
                                else:
                                    caseAuthor = ""
                                FailedCase[subteamOut].append(caseids[0][4:]+"-"+caseAuthor)
                        else:
                            FailedCase[subteamOut].append(ret["name"])
            return FailedCase
        except BaseException as e:
            print(e)
            return dict()

    def collectResult(self, launchID):
        result = self.getLaunchIdWithLaunchName(launchID)
        for launchID in result.keys():
            result[launchID]["caseResult"] = dict()
            failCase = self.getFailCaseID(launchID)
            for subTeam in failCase.keys():
                result[launchID]["caseResult"][subTeam] = failCase[subTeam]
        #print(result)
        return result


    def notifyToSlack(self, notificationList=[]):
        for notificationListSplit in [notificationList[i:i+30] for i in range(0, len(notificationList), 30)]:
            print(notificationListSplit)
            try:
                msgList = []
                for notification in notificationListSplit:
                    msgList.append({"type": "section","text": {"type":"mrkdwn","text": notification}})
                msg = {"blocks": msgList}
                r = self.session.post(url=self.slack_url, json=msg)
                if (r.status_code != 200) and (r.status_code != 201):
                    raise Exception("send slack message error: {0}".format(r.text))
            except BaseException as e:
                print(e)
                print("\n")
        return None

    def collectResultToSlack(self, launchID):
        result = self.collectResult(launchID)
        for testrun in result.keys():
            notificationList = []
            notificationHeader=[]
            notificationHeader.append("******************                    Prow CI Result                     ********************")
            notificationHeader.append("name: "+result[testrun]["name"]+" " +"#"+str(result[testrun]["number"]))
            notificationHeader.append("RP link:" +self.ui_url+str(testrun))
            notificationHeader.append("debug log: "+result[testrun]["description"])
            notificationList.append("\n".join(notificationHeader))
            faildTeamOwner =""
            for subteam in result[testrun]["caseResult"].keys():
                notificationSub=[]
                failedCases = result[testrun]["caseResult"][subteam]
                if failedCases == "":
                   continue
                if self.subteam.lower() != "all":
                    if subteam not in self.subteam.split(":"):
                        continue
                notificationSub.append("---------- subteam: "+subteam+" -------------")
                notificationSub.append("Failed Cases: "+"|".join(result[testrun]["caseResult"][subteam]))
                notificationList.append("\n".join(notificationSub))
                if subteam.replace("_cucushift", "") in self.SUBTEAM_OWNER.keys():
                    if self.SUBTEAM_OWNER[subteam.replace("_cucushift", "")] not in faildTeamOwner:
                        faildTeamOwner = faildTeamOwner + self.SUBTEAM_OWNER[subteam.replace("_cucushift", "")]+" "
            notificationEnd = []
            self.number = self.number+1
            if not self.silence:
                debugMsg = "{0} Please debug failed cases and update the DEFECT TYPE, thanks!".format(faildTeamOwner)
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
        for launchID in self.launchID.split(":"):
            print("collect result of "+launchID)
            self.collectResultToSlack(launchID)

if __name__ == "__main__":
    parser = argparse.ArgumentParser(prog="python3 collect_prow_ci_result_to_slack.py", usage='''%(prog)s -l <launchID> -s <subteam> -t <token> --ignore_investigated''')
    parser.add_argument("-t","--token", default="")
    parser.add_argument("-s","--subteam", default="", help="subteam in g.Describe, separator is colon, eg OLM:OperatorSDK")
    parser.add_argument("-l","--launchID", default="", required=True, help="the launch ID")
    parser.add_argument("-v","--version", default="", help="the release version, eg:4.12")
    parser.add_argument("-c","--cluster", default="", help="the jenkins build number of the cluster for debugging")
    parser.add_argument("-w","--webhook_url", default="", help="the webhook url used to send message")
    parser.add_argument("-g","--group_channel", default="", help="the channel name which will be send result to")
    parser.add_argument("-a","--additional_message", default="", help="additional message")
    parser.add_argument("-d","--days", default="", help="the number of days to collect result")
    parser.add_argument("--upgrade", default=False, action='store_true', help="upgrade test runs")
    parser.add_argument("--ignore_investigated", dest='ignore_investigated', default=False, action='store_true', help="ignore investigated cases")
    parser.add_argument("--silence", dest='silence', default=False, action='store_true', help="the flag to request debug")
    args=parser.parse_args()

    sclient = SummaryClient(args)
    sclient.collectAllResultToSlack()

    exit(0)

