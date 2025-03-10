#!/usr/bin/env python3
import argparse
import requests
from requests.adapters import HTTPAdapter
from urllib3.util import Retry
import urllib3
from urllib3.exceptions import InsecureRequestWarning
from datetime import datetime, timedelta
import yaml
import urllib.parse
class UpdateResultonRP:
    def __init__(self, args):
        self.defectFullNameMap = {
            "PB": "product_bug",
            "AB": "automation_bug",
            "TI": "to_investigate",
            "SI": "system_issue"
            # "SI": "si001",
            # "NI": "si_vf6zbppgm81f"
        }
        self.defectTypeMap = {
            "PB": "pb001",
            "AB": "ab001",
            "TI": "ti001",
            "SI": "si001"
            # "NI": "si_vf6zbppgm81f"
        }
        if args.token == "":
            # currently it is dsiable
            with open("/root/rp.key") as f:
                token_f = yaml.safe_load(f)
                args.token = token_f["ginkgo_rp_mmtoken"]
        urllib3.disable_warnings(category=InsecureRequestWarning)
        self.session = requests.Session()
        self.session.headers["Authorization"] = "bearer {0}".format(args.token)
        self.session.verify = False
        retry = Retry(connect=3, backoff_factor=0.5)
        adapter = HTTPAdapter(max_retries=retry)
        self.session.mount('https://', adapter)
        self.session.mount('http://', adapter)
        self.session.trust_env = False

        self.launch_url = args.endpoint + "/v1/" + args.project + "/launch"
        self.item_url = args.endpoint + "/v1/" + args.project + "/item"
        self.args = args


    def isGolangLaunch(self, attrs):
        isGolang = False
        for attr in attrs:
            if attr["key"] == "launchtype" and attr["value"] == "golang":
            # in the future, maybe add launchtype=cucu and then add check attr["value"] != "cucu"
                isGolang = True
        return isGolang

    def makeLaunchFilterUrl(self, filters=None):
        filter_url = self.launch_url + "?page.page=1&page.size=300"
        if filters["launchName"] != "":
            filter_url = filter_url + "&filter.cnt.name=" + filters["launchName"].replace(" ", "")

        if filters["attributeKey"] != "":
            filter_url = filter_url + "&filter.has.attributeKey=" + urllib.parse.quote(filters["attributeKey"].replace(" ", ""))

        if filters["subTeam"] != "" and filters["attributeValue"] != "":
            att_value = filters["subTeam"] + "," + filters["attributeValue"]
            filter_url = filter_url + "&filter.has.attributeValue=" + urllib.parse.quote(att_value)
        elif filters["subTeam"] != "":
            filter_url = filter_url + "&filter.has.attributeValue=" + urllib.parse.quote(filters["subTeam"])
        elif filters["attributeValue"] != "":
            filter_url = filter_url + "&filter.has.attributeValue=" + urllib.parse.quote(filters["attributeValue"])

        if filters["timeDuration"] != "":
            if len(filters["timeDuration"].split(",")) > 1:
                start_time = filters["timeDuration"].split(",")[0]
                end_time = filters["timeDuration"].split(",")[1]
            else:
                start_time = filters["timeDuration"].split(",")[0]
                end_time = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
            timediff = datetime.strptime(start_time, "%Y-%m-%d %H:%M:%S").strftime('%s.%f')
            sttimestamp = int(float(timediff)*1000)
            timediff = datetime.strptime(end_time, "%Y-%m-%d %H:%M:%S").strftime('%s.%f')
            edtimestamp = int(float(timediff)*1000+1000)
            filter_url = filter_url + "&filter.btw.startTime=" + urllib.parse.quote(str(sttimestamp)+","+str(edtimestamp))

        filter_url = filter_url + "&filter.gte.statistics" + urllib.parse.quote("$executions$failed") + "=" +filters["failedNum"]

        if filters["defectType"] != "":
            url_map = {
                "PB": "&filter.gte.statistics" + urllib.parse.quote("$defects$"+self.defectFullNameMap["PB"]+"$total") + "=1",
                "AB": "&filter.gte.statistics" + urllib.parse.quote("$defects$"+self.defectFullNameMap["AB"]+"$total") + "=1",
                "TI": "&filter.gte.statistics" + urllib.parse.quote("$defects$"+self.defectFullNameMap["TI"]+"$total") + "=1",
                "SI": "&filter.gte.statistics" + urllib.parse.quote("$defects$"+self.defectFullNameMap["SI"]+"$total") + "=1"
                # "SI": "&filter.gte.statistics" + urllib.parse.quote("$defects$system_issue$"+self.defectFullNameMap["SI"]) + "=1",
                # "NI": "&filter.gte.statistics" + urllib.parse.quote("$defects$system_issue$"+self.defectFullNameMap["NI"]) + "=1"
            }
            filter_url = filter_url + url_map[filters["defectType"]]

        print("LaunchFilterURL: {0}".format(filter_url))
        return filter_url


    def getLaunchInfoFromRsp(self, rsp):
        ids = []
        for ret in rsp:
            if not self.isGolangLaunch(ret["attributes"]):
                continue
            ids.append({"id":ret["id"], "name":ret["name"], "number":ret["number"]})
        return ids

    def getLaunches(self, filters=None):
        filter_url = self.makeLaunchFilterUrl(filters)
        ids = []
        total_pages = 0

        try:
            r = self.session.get(url=filter_url)
            if (r.status_code != 200):
                raise Exception("get ID error: {0} with code {1}".format(r.text, r.status_code))
            total_pages = r.json()["page"]["totalPages"]
            ids.extend(self.getLaunchInfoFromRsp(r.json()["content"]))

            for page_number in range(2, total_pages+1):
                filter_url_tmp = filter_url.replace("page.page=1", "page.page="+str(page_number))
                r = self.session.get(url=filter_url_tmp)
                if (r.status_code != 200):
                    print("error to access page number {0} with {1} with code {2}".format(page_number, r.text, r.status_code))
                    print("continue next page, and please rerun it to try failed page")
                    continue
                ids.extend(self.getLaunchInfoFromRsp(r.json()["content"]))

            if len(ids) == 0:
                raise Exception('no matched launch id')
            return ids
        except BaseException as e:
            print(e)
            return None

    def makeItemFilterUrl(self, filters=None):
        filter_url = self.item_url + "?page.page=1&page.size=300&isLatest=false&launchesLimit=0"

        if filters["launch"] != None:
            filter_url = filter_url + "&filter.eq.launchId=" + str(filters["launch"]["id"])

        if filters["itemType"] != "":
            filter_url = filter_url + "&filter.eq.type=" + filters["itemType"]

        if filters["itemStatus"] != "":
            filter_url = filter_url + "&filter.eq.status=" + filters["itemStatus"]

        if filters["defectType"] != "":
            filter_url = filter_url + "&filter.in.issueType=" + self.defectTypeMap[filters["defectType"]]

        # print("ItemFilterURL: {0}".format(filter_url))
        return filter_url

    def getItemInfoFromRsp(self, rsp, filters):
        ids = []

        if len(rsp) == 0:
            print("no case match with status={0} in launch {1} #{2}".format(filters["itemStatus"], filters["launch"]["name"], filters["launch"]["number"]))
            return ids
        for ret in rsp:
            if self.args.author in ret["name"]:
                ids.append({"id":ret["id"], "name":ret["name"]})

        return ids

    def getItems(self, filters):
        query_item_url = self.makeItemFilterUrl(filters)
        ids = []
        total_pages = 0

        r = self.session.get(url=query_item_url)
        if (r.status_code != 200):
            print("can not get case status={0} with error {1} and code {2} for launch {3} #{4}".format(filters["itemStatus"],r.text, r.status_code, filters["launch"]["name"], filters["launch"]["number"]))
            return ids
        total_pages = r.json()["page"]["totalPages"]
        ids.extend(self.getItemInfoFromRsp(r.json()["content"], filters))

        for page_number in range(2, total_pages+1):
            query_item_url_tmp = query_item_url.replace("page.page=1", "page.page="+str(page_number))
            r = self.session.get(url=query_item_url_tmp)
            if (r.status_code != 200):
                print("can not get case status={0} with error {1} and code {2} for launch {3} #{4} for page {5}".format(filters["itemStatus"],r.text, r.status_code, filters["launch"]["name"], filters["launch"]["number"], str(page_number)))
                print("continue next page, and please rerun it to try failed page")
                continue
            ids.extend(self.getItemInfoFromRsp(r.json()["content"], filters))

        return ids

    def updateItemStatus(self, itemid, itemstatus):
        update_tiem_url = self.item_url + "/{0}/update".format(itemid)
        itemdata = {
            "attributes": [
                {
                "key": "manually",
                "value": itemstatus
                }
            ],
            "status": itemstatus
        }
        r = self.session.put(url=update_tiem_url, json=itemdata)
        return r.status_code

    def updateItemPerLaunch(self, launch, itemstatus):
        filters = {
            "launch": launch,
            "itemType": "STEP",
            "defectType": self.args.defecttype,
            "itemStatus": itemstatus
        }
        items = self.getItems(filters)
        for item in items:
            # print(item)
            ret_code = self.updateItemStatus(item["id"], "passed")
            if (ret_code != 200) and (ret_code != 201):
                print("can not change status for case={0} in launch {1} #{2}. please rerun or manually change status".format(item["name"], launch["name"], launch["number"]))

    def ChangeToSuccess(self):
        filters = {
            "launchName": self.args.launchname,
            "timeDuration": self.args.timeduration,
            "subTeam": self.args.subteam,
            "attributeKey": self.args.attrkey,
            "attributeValue": self.args.attrvalue,
            "defectType": self.args.defecttype,
            "failedNum": "1"
        }
        existinglaunchs = self.getLaunches(filters)
        if existinglaunchs == None:
            print("no launch match")
            return
        print("we found launches:\n {0}".format(existinglaunchs))
        for launch in existinglaunchs:
            self.updateItemPerLaunch(launch, "FAILED")


if __name__ == "__main__":
    parser = argparse.ArgumentParser("updateresultrp.py")
    parser.add_argument("-a","--action", default="cls", choices={"cls"}, required=True)
    parser.add_argument("-e","--endpoint", default="https://reportportal-openshift.apps.dno.ocp-hub.prod.psi.redhat.com/api")
    parser.add_argument("-t","--token", default="")
    parser.add_argument("-p","--project", default="ocp")

    parser.add_argument("-l","--launchname", default="")
    parser.add_argument("-s","--subteam", default="")
    parser.add_argument("-td","--timeduration", default="")
    parser.add_argument("-ak","--attrkey", default="")
    parser.add_argument("-av","--attrvalue", default="")
    parser.add_argument("-fn","--failednum", default="0")
    parser.add_argument("-at","--author", default="")
    # parser.add_argument("-dt","--defecttype", default="", choices={"", "PB", "AB", "SI", "NI", "TI"})
    parser.add_argument("-dt","--defecttype", default="", choices={"", "PB", "AB", "SI", "TI"})
    args=parser.parse_args()

    updr = UpdateResultonRP(args)
    if args.action == "cls":
        updr.ChangeToSuccess()
    exit(0)

