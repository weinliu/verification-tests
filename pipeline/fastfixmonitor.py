#!/usr/bin/env python3
import argparse
import requests
from requests.adapters import HTTPAdapter
from urllib3.util import Retry
import urllib3
from urllib3.exceptions import InsecureRequestWarning
from datetime import datetime, timedelta, timezone
import os
import yaml
import re

class BugzillaClient:
    def __init__(self, args):
        urllib3.disable_warnings(category=InsecureRequestWarning)
        self.session = requests.Session()
        self.session.headers["Content-type"] = "application/json"
        self.session.headers["Accept"] = "application/json"
        self.session.headers["Authorization"] = "Bearer {0}".format(args.apikey)
        self.session.verify = False
        retry = Retry(connect=3, backoff_factor=0.5)
        adapter = HTTPAdapter(max_retries=retry)
        self.session.mount('https://', adapter)
        self.session.mount('http://', adapter)
        self.session.trust_env = False

        self.bug_url = args.endpoint + "/bug"
        self.slack_url = args.slack + args.slacktoken.replace("-", "/")
        self.args = args
        # print (self.session.headers)

        if os.path.exists("/tmp/fast_fix.json"):
            os.remove("/tmp/fast_fix.json")

    def updateByID(self, bid, attr_key, attr_value):
        try:
            datas = { attr_key: attr_value}

            r = self.session.put(url=self.bug_url+"/"+str(bid), json=datas)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("update bug error: {0}".format(r.text))
            return r.status_code
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def updateQaWhiteBoard(self, bid, contents):
        try:

            r = self.session.get(url=self.bug_url+"/"+str(bid)+"?include_fields=_default")
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("get bug error in updateQaWhiteBoard: {0}".format(r.text))

            return self.updateByID(bid, "cf_qa_whiteboard", r.json()["bugs"][0]["cf_qa_whiteboard"] + "\r\n" + contents)
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def removeNotifyWhiteBoard(self, bid):
        try:

            r = self.session.get(url=self.bug_url+"/"+str(bid)+"?include_fields=_default")
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("get bug error in removeNotifyWhiteBoard: {0}".format(r.text))
            cf_qa_whiteboard = r.json()["bugs"][0]["cf_qa_whiteboard"]

            if not self.isAlreadyNotify(cf_qa_whiteboard, str(bid)):
                return r.status_code

            newContent = ""
            for line in cf_qa_whiteboard.split("\r\n"):
                notifyTime = re.findall(r'Already notify QA to pre-verify it {0} at UTC'.format(str(bid)), line)
                if len(notifyTime) == 0:
                    newContent = newContent + "\r\n" + line

            return self.updateByID(bid, "cf_qa_whiteboard", newContent)
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def getByID(self, bid):
        try:
            include_fields = "id,cf_verified,cf_qa_whiteboard,last_change_time,external_bugs,keywords,qa_contact,status,assigned_to"

            # r = self.session.get(url=self.bug_url+"/"+str(bid)+"?include_fields=_extra,_default", params=params)
            r = self.session.get(url=self.bug_url+"/"+str(bid)+"?include_fields="+include_fields)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("get bug error: {0}".format(r.text))
            # print(r.json())
            bugs = r.json()["bugs"]
            bugs_info = {}
            for bug in bugs:
                bugs_info[str(bid)] = {"id": str(bid),
                                        "cf_verified":bug["cf_verified"],
                                        "cf_qa_whiteboard":bug["cf_qa_whiteboard"],
                                        "qa_contact_detail":bug["qa_contact_detail"],
                                        "last_change_time":bug["last_change_time"],
                                        "external_bugs":bug["external_bugs"],
                                        "keywords":bug["keywords"],
                                        "status":bug["status"],
                                        "assigned_to_detail":bug["assigned_to_detail"]}
            return bugs_info
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def getCommentsById(self, bid):
        try:

            r = self.session.get(url=self.bug_url+"/"+str(bid)+"/comment")
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("get comments error: {0}".format(r.text))
            # print(r.json())
            comments = r.json()["bugs"][str(bid)]["comments"]
            # print(comments)
            comments_info = []
            for comment in comments:
                comments_info.append({"creator":comment["creator"], "creation_time":comment["creation_time"]})
            return comments_info
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def getByFilter(self, filters):
        try:
            bug_list = []

            # status = "bug_status=NEW&bug_status=ASSIGNED&bug_status=POST&bug_status=MODIFIED&bug_status=ON_DEV&bug_status=ON_QA"
            if filters["status"] != "":
                status = ""
                statuses = filters["status"].split(",")
                for s in statuses:
                    status = status + "bug_status="+ s.strip()+ "&"
                status = status[:-1]

            # keywords = "keywords=TestBlocker%2C%20&keywords_type=allwords"
            if filters["keywords"] != "":
                keywords = "keywords="+filters["keywords"].strip()+"%2C%20&keywords_type=allwords"

            classification = "classification=Red%20Hat"
            product = "product=OpenShift%20Container%20Platform"
            search_filter = status+"&"+classification+"&"+keywords+"&"+product

            r = self.session.get(url=self.bug_url+"?"+search_filter+"&"+"include_fields=id")
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                raise Exception("get bug by filter error: {0}".format(r.text))
            bugs = r.json()["bugs"]
            for bug in bugs:
                bug_list.append(bug["id"])
            return bug_list
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def prExists(self, external_bugs):
        existing = False
        for external_bug in external_bugs:
            if external_bug["type"]["type"] == "GitHub":
                existing = True
        return existing

    def isPreVerified(self, cf_verified):
        if "Tested" in cf_verified:
            return True
        return False

    def isAlreadyNotify(self, cf_qa_whiteboard, id):
        if "Already notify QA to pre-verify it {0}".format(id) in cf_qa_whiteboard:
            return True
        return False

    def isAlreadyQueryDev(self, cf_qa_whiteboard, id):
        if "Already notify QA to contact with Dev for PR of bug {0}".format(id) in cf_qa_whiteboard:
            return True
        return False

    def notifyQaInSlack(self, slack_list):
        try:
            if len(slack_list) == 0:
                return None

            qa_list = {}
            for bug in slack_list:
                if bug["qa"] in qa_list:
                    qa_list[bug["qa"]].append(bug["id"])
                else:
                    qa_list[bug["qa"]] = [bug["id"]]

            notification = ""
            for qa, ids in qa_list.items():
                bugs = ""
                for id in ids:
                    bugs = bugs + "https://bugzilla.redhat.com/show_bug.cgi?id=" + id + "\n"
                # notification = notification + "Hi @{0}, please finish the verification of fastfix bugs in one day per process required:\n{1}Thanks!\n".format("kuiwang", bugs)
                notification = notification + "Hi @{0}, please finish the verification of fastfix bugs in one day per process required:\n{1}Thanks!\n".format(qa.split("@")[0], bugs)
            notification = notification + "\nFor How, please refer to https://docs.google.com/document/d/1qSVZtKR4TGsrZjj-IDNW3IN40YicbZm9F2UzFTy1qvI/edit?usp=sharing \n"
            notification = notification +  "cc @{0}\n".format("openshift-qe-lead")
            msg = {"blocks": [{"type": "section","text": {"type":"mrkdwn","text": notification}}]}

            r = self.session.post(url=self.slack_url, json=msg)
            # print(r.status_code)
            # print(r.text)
            if (r.status_code != 200) and (r.status_code != 201):
                for bug in slack_list:
                    self.removeNotifyWhiteBoard(bug["id"])
                raise Exception("send slack message error: {0}".format(r.text))
            return r.status_code
        except BaseException as e:
            print(e)
            print("\\n")
            return None

    def getUtctimestamp(self):
        return datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%S.%f')

    def isWeekend(self, offset):
        timezone_offset = offset
        tz = timezone(timedelta(hours=timezone_offset))
        weekend = datetime.now(tz).weekday() > 4
        print("weekend: {0}".format(str(weekend)))
        return weekend

    def getIntervalInHalfHour(self, notifytime):
        notifyTime = datetime.strptime(notifytime, "%Y-%m-%dT%H:%M:%S.%f")
        currentTime = datetime.strptime(self.getUtctimestamp(), "%Y-%m-%dT%H:%M:%S.%f")
        timedelta1 = currentTime + timedelta(minutes=10) - notifyTime
        return divmod(timedelta1.days * 24 * 60 * 60 + timedelta1.seconds, 1800)[0]

    def getNotifyTime(self, cf_qa_whiteboard, id):
        notifyTimeList = []
        for line in cf_qa_whiteboard.split("\r\n"):
            notifyTime = re.findall(r'Already notify QA to pre-verify it {0} at UTC (.*)'.format(id), line)
            if len(notifyTime) != 0:
                notifyTimeList.append(notifyTime[0])
        if len(notifyTimeList) == 0:
            return None
        return notifyTimeList[0]

    def isSendTrackEmail(self, id, cf_qa_whiteboard, interval, offset):
        notifyTime = self.getNotifyTime(cf_qa_whiteboard, id)
        if notifyTime == None:
            return False

        intervalHalfHour = self.getIntervalInHalfHour(notifyTime)
        if intervalHalfHour == 0:
            return False

        if intervalHalfHour < offset:
            return False
        print("intervalHalfHour is {0}".format(intervalHalfHour))
        return (intervalHalfHour - offset) % interval == 0

    def isCommented(self,id, qa, cf_qa_whiteboard):
        comments_info = self.getCommentsById(id)
        if comments_info == None:
            return True
        if len(comments_info) == 0:
            return False

        notifyTime = self.getNotifyTime(cf_qa_whiteboard, str(id))
        if notifyTime == None:
            return True

        commented = False
        first_notify = datetime.strptime(notifyTime, "%Y-%m-%dT%H:%M:%S.%f")
        for comment in comments_info:
            if comment["creator"] == qa and (datetime.strptime(comment["creation_time"], "%Y-%m-%dT%H:%M:%SZ") - first_notify).total_seconds() > 0:
                commented = True
        return commented

    def checkBug(self, id):
        bug_info = self.getByID(id)
        if bug_info == None or len(bug_info) == 0:
            return None

        if not self.prExists(bug_info[str(id)]["external_bugs"]):
            if bug_info[str(id)]["status"] in ["NEW", "ASSIGNED"]:
                print("bug {0} has no PR yet, so we do not verify it currently".format(str(id)))
                return None

            if self.isAlreadyQueryDev(bug_info[str(id)]["cf_qa_whiteboard"], str(id)):
                print("bug {0} has no PR yet, but it is in POST or later status. already notify QA to contact Dev for PR.".format(str(id)))
                return None

            if self.updateQaWhiteBoard(id, "Already notify QA to contact with Dev for PR of bug {0} at UTC {1}".format(str(id), self.getUtctimestamp())) is None:
                print("fail to update bug {0} with notify QA to contact with Dev for PR, and will notify qa next time ".format(str(id)))
                return None
            print("bug {0} has no PR yet, but it is in POST or later status. Notify QA to contact Dev for PR.".format(str(id)))
            return {"id": str(id), "qa":bug_info[str(id)]["qa_contact_detail"]["email"],
                    "notify":"contactdevforpr", "assignee":bug_info[str(id)]["assigned_to_detail"]["email"]}

        if self.isPreVerified(bug_info[str(id)]["cf_verified"]):
            print("bug {0} is already pre-verified".format(str(id)))
            return None

        if self.isAlreadyNotify(bug_info[str(id)]["cf_qa_whiteboard"], str(id)):
            print("already notify qa to pre-verify bug {0} with slack".format(str(id)))
            if self.isSendTrackEmail(str(id), bug_info[str(id)]["cf_qa_whiteboard"], 96, 48) and not self.isCommented(id, bug_info[str(id)]["qa_contact_detail"]["email"], bug_info[str(id)]["cf_qa_whiteboard"]):
                print("no comment on bug {0} from qa contactor within 24 hours after notification".format(str(id)))
                return {"id": str(id), "qa":bug_info[str(id)]["qa_contact_detail"]["email"],
                        "notify":"escalatetrack", "assignee":bug_info[str(id)]["assigned_to_detail"]["email"]}

            if self.isSendTrackEmail(str(id), bug_info[str(id)]["cf_qa_whiteboard"], 24, 0) and not self.isWeekend(8):
                print("notify qa to pre-verify bug {0} with email becuse it is not first time notification".format(str(id)))
                return {"id": str(id), "qa":bug_info[str(id)]["qa_contact_detail"]["email"],
                        "notify":"continoustrack", "assignee":bug_info[str(id)]["assigned_to_detail"]["email"]}
            return None

        if self.updateQaWhiteBoard(id, "Already notify QA to pre-verify it {0} at UTC {1}".format(str(id), self.getUtctimestamp())) is None:
            print("fail to update bug {0}, and will notify qa next time ".format(str(id)))
            return None

        print("notify qa to pre-verify bug {0} with slack becuse it is first time notification".format(str(id)))
        return {"id": str(id), "qa":bug_info[str(id)]["qa_contact_detail"]["email"],
                "notify":"slack", "assignee":bug_info[str(id)]["assigned_to_detail"]["email"]}

    def handleBugs(self):

        filters = {
            "status": self.args.status,
            "keywords": self.args.keywords
        }

        bug_list = self.getByFilter(filters)
        if bug_list is None:
            print("fail to get bug id. try it in hours")
            return
        if len(bug_list) == 0:
            print("no fastfix bug is found")
            return

        all_list = []
        slack_list = []
        for id in bug_list:
            bug_info = self.checkBug(id)
            if bug_info != None and bug_info["notify"] == "slack":
                slack_list.append(bug_info)
            if bug_info != None:
                all_list.append(bug_info)
        print("--------------------------------------------------")
        print(all_list)
        print("--------------------------------------------------")
        with open('/tmp/fast_fix.json', 'w') as fp:
            yaml.dump(all_list, fp)
        print("--------------------------------------------------")
        print(slack_list)
        print("--------------------------------------------------")
        self.notifyQaInSlack(slack_list)


if __name__ == "__main__":
    parser = argparse.ArgumentParser("fastfixmonitor.py")
    parser.add_argument("-a","--action", default="id", choices={"id", "search"}, required=True)
    parser.add_argument("-e","--endpoint", default="https://bugzilla.redhat.com/rest")
    parser.add_argument("-k","--apikey", default="")   #openshift-qe-notifier@bot.bugzilla.redhat.com
    parser.add_argument("-sl","--slack", default="https://hooks.slack.com/services/")
    parser.add_argument("-st","--slacktoken", default="") #forum-qe
    parser.add_argument("-id","--bugid", default="1874026")
    parser.add_argument("-s","--status", default="NEW,ASSIGNED,POST,MODIFIED,ON_DEV,ON_QA")
    parser.add_argument("-w","--keywords", default="FastFix")
    args=parser.parse_args()

    bc = BugzillaClient(args)
    exit(0)
    # because bugzilla is not used for OCP and api key of openshift-qe-notifier@bot.bugzilla.redhat.com expires

    if args.action == "id":
        print(bc.checkBug(args.bugid))
    if args.action == "search":
        bc.handleBugs()
    exit(0)

