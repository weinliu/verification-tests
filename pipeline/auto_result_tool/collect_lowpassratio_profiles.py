#!/usr/bin/python3
# author : xzha
import os
import re
import urllib3
import requests
import argparse
import json
import logging
import pprint
import time
from urllib3.exceptions import InsecureRequestWarning
from requests.adapters import HTTPAdapter
from urllib3.util import Retry
from datetime import date, datetime
import gspread
from jira import JIRA
from oauth2client.service_account import ServiceAccountCredentials
urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

def get_logger(filePath):
    logger = logging.getLogger('my_logger')
    logger.setLevel(logging.DEBUG)
    #fh = logging.FileHandler(filePath)
    #fh.setLevel(logging.DEBUG)
    sh = logging.StreamHandler()
    sh.setLevel(logging.INFO)
    formatter = logging.Formatter(fmt='%(asctime)s %(lineno)d %(message)s',
                                  datefmt='%Y-%m-%d-%H:%M:%S')
    #fh.setFormatter(formatter)
    sh.setFormatter(formatter)
    #logger.addHandler(fh)
    logger.addHandler(sh)
    return logger

class SummaryClient:
    def __init__(self, args):
        self.logFile = args.log
        if not self.logFile:
            self.logFile = os.path.join(os.path.abspath(os.path.dirname(__file__)), "collect_case_result.log")
        if os.path.isfile(self.logFile):
            os.remove(self.logFile)
        self.logger = get_logger(self.logFile)
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

        self.version = args.version
        self.sub_team = args.subteam
        self.parent_jira_issue = args.parent_jira
        self.profile_filter = args.profile
        self.assginee = args.assginee
        
        self.base_url = "https://reportportal-openshift.apps.ocp-c1.prod.psi.redhat.com"
        self.launch_url = self.base_url +"/api/v1/prow/launch"
        self.item_url = self.base_url + "/api/v1/prow/item"
        self.ui_url = self.base_url + "/ui/#prow/launches/all/"
        self.ocp_launch_url = self.base_url +"/api/v1/ocp/launch"
        self.ocp_item_url = self.base_url + "/api/v1/ocp/item"
        self.ocp_ui_url = self.base_url + "/ui/#ocp/launches/all/"
        self.days = args.days
        self.cases_result = dict()
        self.jiraManager = None
        
        if self.parent_jira_issue:
            self.jira_token = args.jira_token
            if not self.jira_token:
                raise BaseException("ERROR: jira token is empty, please input the jira token using --jira_token")
            self.jiraManager = JIRAManager("https://issues.redhat.com", self.jira_token, self.logger)

    def get_prow_case_result(self):
        day_number = self.days
        filter_url = self.launch_url + '?filter.has.compositeAttribute=version:{0}&filter.btw.startTime=-{1};1440;-0000&page.size=2000&page.page=1'.format(self.version, str(1440*day_number))
        self.logger.debug("filter_url is "+filter_url)
        try:
            launch_result = self.session.get(url=filter_url)
            if (launch_result.status_code != 200):
                self.logger.error("get items error: {0}".format(launch_result.text))
            if len(launch_result.json()["content"]) == 0:
                return ''
            self.logger.debug(json.dumps(launch_result.json(), indent=4, sort_keys=True))
            total_pages_launch = launch_result.json()["page"]["totalPages"]
            self.logger.info("total page is %s", total_pages_launch)
            for page_number_launch in range(1, total_pages_launch+1):
                self.logger.info("page is %s", page_number_launch)
                r = self.session.get(url=filter_url)
                if (r.status_code != 200):
                    self.logger.error("get launch error: {0}".format(r.text))
                self.logger.debug(json.dumps(r.json(), indent=4, sort_keys=True))
                if len(r.json()["content"]) == 0:
                    self.logger.debug("no launch found")
                if page_number_launch == 1:
                    content = launch_result.json()["content"]
                else:
                    filter_url_tmp = filter_url.replace("page.page=1", "page.page="+str(page_number_launch))
                    launch_result_tmp = self.session.get(url=filter_url_tmp)
                    self.logger.info(filter_url_tmp)
                    if (launch_result_tmp.status_code != 200):
                        self.logger.error("get item case error: {0}".format(launch_result_tmp.text))
                    if len(launch_result_tmp.json()["content"]) == 0:
                        break
                    self.logger.debug(json.dumps(launch_result_tmp.json(), indent=4, sort_keys=True))
                    content = launch_result_tmp.json()["content"]

                for ret in content:
                    if not ret["statistics"]["executions"]:
                        continue
                    name = ret["name"]
                    description = ret["description"]
                    build_version = ''
                    profilename = ''
                    installationStatus = ''
                    for attribute in ret['attributes']:
                        if attribute['key'] == 'version_installed':
                            build_version = attribute['value']
                        if attribute['key'] == 'profilename':
                            profilename = attribute['value']
                        if attribute['key'] == 'install':
                            installationStatus = attribute['value']
                    if self.profile_filter:
                        if self.profile_filter not in profilename:
                            continue
                    
                    self.logger.info("")
                    self.logger.debug("get result for: %s", name)
                    if installationStatus == "fail":
                        self.logger.info("%s installation is fail, skip", profilename)
                        continue
                    failed = 0
                    passed = 0
                    if "failed" in ret["statistics"]["executions"].keys():
                        failed = int(ret["statistics"]["executions"]["failed"])
                    if "passed" in ret["statistics"]["executions"].keys():
                        passed = int(ret["statistics"]["executions"]["passed"])
                    if (passed+failed) == 0:
                        self.logger.info("%s passed and failed cases are 0, skip", profilename)
                        continue
                    passratio = float(passed)/(failed+passed)
                    if passratio > 0.9:
                        self.logger.info("%s passratio is %s, skip", profilename, passratio)
                        continue
                    
                    id = ret["id"]
                    link = self.ui_url+str(id)
                    
                    time.sleep(1)
                    
                    item_url_profile = self.item_url + "?filter.eq.launchId={0}&launchesLimit=0&page.size=2000&page.page=1".format(id)
                    self.logger.debug(item_url_profile)
                    try:
                        launch_result_profile = self.session.get(url=item_url_profile)
                        if (launch_result_profile.status_code != 200):
                            self.logger.error("get item case error: {0}".format(launch_result_profile.text))
                        if len(launch_result_profile.json()["content"]) == 0:
                            return ''
                        self.logger.debug(json.dumps(launch_result_profile.json(), indent=4, sort_keys=True))
                        total_pages_profile = launch_result_profile.json()["page"]["totalPages"]
                        
                        for page_number_profile in range(1, total_pages_profile+1):
                            if page_number_profile == 1:
                                content_profile = launch_result_profile.json()["content"]
                            else:
                                item_url_profile_tmp = item_url_profile.replace("page.page=1", "page.page="+str(page_number_profile))
                                launch_result_profile_tmp = self.session.get(url=item_url_profile_tmp)
                                if (launch_result_profile_tmp.status_code != 200):
                                    self.logger.error("get item case error: {0}".format(launch_result_profile_tmp.text))
                                if len(launch_result_profile_tmp.json()["content"]) == 0:
                                    break
                                self.logger.debug(json.dumps(launch_result_profile_tmp.json(), indent=4, sort_keys=True))
                                content_profile = launch_result_profile_tmp.json()["content"]
                            for ret_profile in content_profile:
                                if ret_profile["type"] == "SUITE":
                                    subteam_name = ret_profile["name"]
                                    if self.sub_team and subteam_name not in self.sub_team:
                                        continue
                                    else:
                                        failed = 0
                                        passed = 0
                                        if "failed" in ret_profile["statistics"]["executions"].keys():
                                            failed = int(ret_profile["statistics"]["executions"]["failed"])
                                        if "passed" in ret_profile["statistics"]["executions"].keys():
                                            passed = int(ret_profile["statistics"]["executions"]["passed"])
                                        if (failed+passed) == 0:
                                            continue
                                        passratio = float(passed)/(failed+passed)
                                        create_jira_issue = False
                                        if "destructive" in name:
                                            if passratio < 0.5:
                                                create_jira_issue = True
                                        else:
                                            if passratio < 0.90:
                                                create_jira_issue = True
                                        if create_jira_issue:
                                            comments = os.linesep.join(["profile: "+name,
                                                        "{0}: pass: {1}, failed: {2}".format(subteam_name, passed, failed),
                                                        "RP link: "+link,
                                                        "debug log: "+description])
                                            self.logger.info("%spass: %s, failed: %s, pass ratio %s", subteam_name, passed, failed, passratio)
                                            self.logger.info("Create jira ticket for %s profile %s with build %s", subteam_name, profilename, build_version)
                                            self.logger.info(comments)
                                            if self.jiraManager:
                                                self.jiraManager.create_sub_task(self.parent_jira_issue, subteam_name, build_version, profilename, comments, self.assginee)
                        self.logger.debug(json.dumps(self.cases_result, indent=4, sort_keys=True))
                    except BaseException as e:
                        self.logger.error(e)

            self.logger.debug(self.cases_result.keys())
            return self.cases_result
        except BaseException as e:
            print(e)
            return dict()     
        
class JIRAManager:
    def __init__(self, jira_server, token_auth, logger):
        self.logger = logger
        options = {
            'server': jira_server,
            'verify': True 
        }            
        self.jira = JIRA(options=options, token_auth=token_auth)
        
    def get_subtask_list(self, parent_jira):
        issues = dict()
        issue = self.jira.issue(parent_jira)
        for issue in issue.fields.subtasks:
            issues[issue.key] = dict()
            issues[issue.key]["summary"] = issue.fields.summary
            issues[issue.key]["link"] = "https://issues.redhat.com/browse/"+issue.key
            
        self.logger.debug(pprint.pformat(issues, indent=1))
        return issues
    
    def create_sub_task(self, parent_jira,subteam_name, buildversion, profile, comments, assginee):
        description_str = """
Hi, 
{profile} has low pass ratio, please help to check it.
{comments}
""".format(profile=profile,comments=comments)
        self.logger.info("Create sub task for %s", profile)
        
        parent_issue = self.jira.issue(parent_jira)
        project_key = parent_issue.fields.project.key
        parent_issue_key = parent_issue.key
        subtasks = self.get_subtask_list(parent_jira)

        for substask in subtasks.keys():
            summary = subtasks[substask]["summary"]
            if buildversion.lower() in summary.lower() and profile.lower() in summary.lower() and subteam_name.lower() in summary.lower():
                self.logger.info("add comments to %s", substask)
                self.jira.add_comment(substask, description_str)
                case_issue = self.jira.issue(substask)
                if not case_issue.fields.customfield_12310243:
                    self.logger.info("update Story Points")
                    case_issue.update(fields={'customfield_12310243': 1.0})
                if case_issue.fields.status.name in ['Closed']:
                    self.jira.transition_issue(case_issue, transition='NEW')
                return substask
      
        subtask = self.jira.create_issue(
                        project=project_key,
                        summary="[LOW PASS RATE] ["+subteam_name+"] "+ buildversion+': '+profile,
                        description=description_str,
                        issuetype={'name': 'Sub-task'},
                        parent={'key': parent_issue_key},
                        assignee= {"name": assginee}
        )
        subtask.update(fields={'customfield_12310243': 1.0})

        self.logger.info("--------- Sub-task %s is created SUCCESS ----------", subtask.key)
        self.logger.debug(json.dumps(subtask.raw['fields'], indent=4, sort_keys=True))
        return subtask.key
       

########################################################################################################################################
if __name__ == "__main__":
    parser = argparse.ArgumentParser(prog="python3 collect_result.py", usage='''%(prog)s''')
    parser.add_argument("-t","--token", default="", required=False, help="the token of the RP")
    parser.add_argument("-s", "--subteam", default="OLM", required=False, help="the sub team name")
    parser.add_argument("-log","--log", default="", required=False, help="the log file")
    parser.add_argument("-v", "--version", default='4.14', required=False, help="the ocp version")
    parser.add_argument("-d", "--days", default=7, type=int, required=False, help="the days number")
    parser.add_argument("-p", "--parent_jira", default="", required=False, help="the parent jira issue link")
    parser.add_argument("-a", "--assginee", default="rhn-support-xzha", required=False, help="the assginee of the sub-task")
    parser.add_argument("-jt", "--jira_token", default="", required=False, help="the jira token")
    parser.add_argument("--profile", default="", required=False, help="the profiles name")

    
    args=parser.parse_args()

    sclient = SummaryClient(args)
    #sclient.create_sub_jira_task_all()
    #sclient.collectResult()
    sclient.get_prow_case_result()

    #task_list = sclient.jiraManager.get_subtask_list("OCPQE-23411")
    #sclient.jiraManager.create_sub_task(None, "OCPQE-23411", task_list, "OCP-45402", "OLM opm should opm validate should detect cycles in channels", "xzha", "test")
    
    exit(0)

    

    
