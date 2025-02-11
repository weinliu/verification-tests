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

class UnifyciRPClient:
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
        # os.environ['no_proxy'] = "reportportal-openshift.apps.ocp4.prod.psi.redhat.com"
        self.session.trust_env = False

        self.launch_url = args.endpoint + "/v1/" + args.project + "/launch"
        self.item_url = args.endpoint + "/v1/" + args.project + "/item"
        self.log_url = args.endpoint + "/v1/" + args.project + "/log"
        self.args = args
        # print (self.session.headers)

    def logResult(self):
        try:
            return self.importResult()
        except BaseException as e:
            print(e)
            print("\\n")
            return False


    def importResult(self):
        import_url = self.launch_url + "/import"
        # print(import_url)
        try:
            files = {'file': (self.args.file, open(self.args.file,'rb'), 'application/zip')}
            r = self.session.post(url=import_url, files=files, timeout=120)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200):
                raise Exception("import error: {0}".format(r.text))

            id = self.getLaunchIdWithLaunchUuid(r.json()["message"].split("id =")[1].strip().split(" ")[0])
            if id == None:
                raise Exception("can not get id for new launch")

            buildVersion = self.getAttrOption("build_version")
            if buildVersion == None or buildVersion == "":
                buildVersion = "nobuildversion"
            pipelineType = "unifyci"

            attDict = {
            "name":     {"action": "add", "value":os.path.splitext(os.path.basename(self.args.file))[0]},
            "team":     {"action": "add", "value":self.args.subteam},
            "version":  {"action": "add", "value":self.args.version.replace(".", "_")},
            "build_version":  {"action": "add", "value":buildVersion},
            "pipeline_type":  {"action": "add", "value":pipelineType},
            "profilename": {"action": "add", "value":self.args.profilename},
            "launchtype": {"action": "add", "value":"golang"},
            }
            attDict["trial"] = {"action": "add", "value":"\"\""}
            if not self.handleLaunchAttribution([id], attDict):
                raise Exception("fail to add attrs")

            return True
        except BaseException as e:
            print(e)
            print("\\n")
            return False


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


if __name__ == "__main__":
    parser = argparse.ArgumentParser("unifycirp.py")
    parser.add_argument("-a","--action", default="import", choices={"import", "attr"}, required=True)
    parser.add_argument("-e","--endpoint", default="https://reportportal-openshift.apps.ocp4.prod.psi.redhat.com/api")
    parser.add_argument("-t","--token", default="")
    parser.add_argument("-ta","--tatoken", default="")
    parser.add_argument("-p","--project", default="ocp")
    #import
    parser.add_argument("-f","--file", default="")
    parser.add_argument("-s","--subteam", default="")
    parser.add_argument("-v","--version", default="")
    parser.add_argument("-ao","--attroption", default="")
    parser.add_argument("-pn","--profilename", default="cluster_profile")
    #handle attr
    parser.add_argument("-id","--launchid", default="")
    parser.add_argument("-aa","--attract", default="")
    parser.add_argument("-key","--attrkey", default="")
    parser.add_argument("-value","--attrvalue", default="")
    args=parser.parse_args()

    rpc = UnifyciRPClient(args)
    if args.action == "import":
        if rpc.logResult():
            print("SUCCESS")
        else:
            print("FAIL")

    if args.action == "attr":
        attDict = {args.attrkey: {"action":args.attract, "value":args.attrvalue}}
        if rpc.handleLaunchAttribution([args.launchid], attDict):
            print("SUCCESS")
        else:
            print("FAIL")
    exit(0)

