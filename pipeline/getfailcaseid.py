#!/usr/bin/env python3
import argparse
import requests
from requests.adapters import HTTPAdapter
from urllib3.util import Retry
import urllib3
from urllib3.exceptions import InsecureRequestWarning
import yaml
import re

class ReportPortalClient:
    def __init__(self, args):
        urllib3.disable_warnings(category=InsecureRequestWarning)
        self.session = requests.Session()
        self.session.headers["Authorization"] = "bearer {0}".format(args.token)
        self.session.verify = False
        retry = Retry(connect=3, backoff_factor=0.5)
        adapter = HTTPAdapter(max_retries=retry)
        self.session.mount('https://', adapter)
        self.session.mount('http://', adapter)

        self.launch_url = "https://reportportal-openshift.apps.dno.ocp-hub.prod.psi.redhat.com/api/v1/ocp/launch"
        self.item_url = "https://reportportal-openshift.apps.dno.ocp-hub.prod.psi.redhat.com/api/v1/ocp/item"
        self.args = args
        # print (self.session.headers)

    def getLaunchIdWithLaunchName(self, launchname, attrfilter=None):
        filter_url = self.launch_url + "?filter.eq.name=" + launchname
        # print(filter_url)
        try:
            r = self.session.get(url=filter_url)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("get launch error: {0}".format(r.text))
            ids = []
            if len(r.json()["content"]) == 0:
                raise Exception("no launch found by name: {0}".format(launchname))
            for ret in r.json()["content"]:
                for attr in ret["attributes"]:
                    if attr["key"] == attrfilter["key"] and attr["value"] == attrfilter["value"]:
                        ids.append(ret["id"])
            if len(ids) != 1:
                raise Exception("the launch should be only one, but not: {0}. Please check your launchname and subteam are correct".format(ids))
            return ids[0]
        except BaseException as e:
            print(e)
            return None

    def getFailCaseID(self):
        launchId = self.getLaunchIdWithLaunchName(self.args.launchname, {"key": "team", "value":self.args.subteam})
        if launchId is None:
            return "no Launch is found"

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
            for ret in r.json()["content"]:
                if ret["type"] == "STEP":
                    caseids = re.findall(r'OCP-\d{4,}', ret["name"])
                    if len(caseids) > 0:
                        caseidList.append(caseids[0][4:])
            if len(caseidList) == 0:
                raise Exception("can not find matched case ID. maybe your case title has no case ID")
            separator = '|'
            return separator.join(caseidList)
        except BaseException as e:
            print(e)

if __name__ == "__main__":
    parser = argparse.ArgumentParser(prog="python3 getfailcaseid.py", usage='%(prog)s -l <launchname> -s <subteam>')
    parser.add_argument("-t","--token", default="4388c1c6-98e8-4e5d-8923-8dffa84a6425") #it is example token for trial project
    parser.add_argument("-s","--subteam", default="", required=True, help="please input subteam you made in your case g.Describe")
    parser.add_argument("-l","--launchname", default="", required=True, help="please input lauchname which is the value of LAUNCH_NAME of the job")
    args=parser.parse_args()

    rpc = ReportPortalClient(args)
    print("The tool is used to get failed case ID in case you want to rerun these case, so just COPY&PASTE output into SCENARIO of job")
    print("NOTE: only case title with case ID could be got.\n")
    print(rpc.getFailCaseID())
    
    exit(0)

