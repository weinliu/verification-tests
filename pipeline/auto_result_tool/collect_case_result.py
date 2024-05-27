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

        self.key_file = args.key
        if not self.key_file:
            self.key_file = '/Users/zhaoxia/test/PROW/collect_result/key.json'
        self.version = args.version
        self.gclient = self.getclient()
        self.target_file = args.file
        self.e2e_sheet = self.version
        self.sub_team = args.subteam
        self.parent_jira_issue = args.parent_jira
        self.sheet_name = args.sheet
        self.author_map_file = args.author
        self.skip_no_failure_cases = args.skip_no_failure_cases
        self.pass_rate_threshold = args.pass_rate_threshold
        
        self.base_url = "https://reportportal-openshift.apps.ocp-c1.prod.psi.redhat.com"
        self.launch_url = self.base_url +"/api/v1/prow/launch"
        self.item_url = self.base_url + "/api/v1/prow/item"
        self.ui_url = self.base_url + "/ui/#prow/launches/all/"
        self.ocp_launch_url = self.base_url +"/api/v1/ocp/launch"
        self.ocp_item_url = self.base_url + "/api/v1/ocp/item"
        self.ocp_ui_url = self.base_url + "/ui/#ocp/launches/all/"
        self.days = args.days
        self.platfrom_list = ["aws", "gcp", "vsphere", "azure", "baremetal", "alibaba", "ibmcloud", "nutanix", "osp", "powervs"]
        self.cases_result = dict()
        self.jiraManager = None
        
        if self.parent_jira_issue:
            self.jira_token = args.jira_token
            if not self.jira_token:
                raise BaseException("ERROR: jira token is empty, please input the jira token using --jira_token")
            self.jiraManager = JIRAManager("https://issues.redhat.com", self.jira_token, self.logger)


    def getclient(self):
        scope = ['https://spreadsheets.google.com/feeds', 'https://www.googleapis.com/auth/drive']
        creds = ServiceAccountCredentials.from_json_keyfile_name(self.key_file, scope)
        return gspread.authorize(creds)
    
    def get_platform(self, profile):
        profile_lower = profile.lower()
        for platfrom_index in self.platfrom_list:
            if platfrom_index in profile_lower:
                self.logger.debug("platfrom is %s", platfrom_index)
                return platfrom_index
        return ''
    
    def get_jenkins_platform(self, profile):
        profile_lower = profile.lower()
        for platfrom_index in self.platfrom_list:
            if platfrom_index in profile_lower:
                self.logger.debug("platfrom is %s", platfrom_index)
                return platfrom_index
        if "metal" in profile_lower:
            return "baremetal"
        if "osp" in profile_lower:
            return "osp"
        if "packet" in profile_lower:
            return "baremetal"
        return ''
    
    def get_architecture(self, build_version):
        build_version_lower = build_version.lower()
        if "arm" in build_version_lower:
            return "arm64"
        if "multi" in build_version_lower:
            return "multi" 
        return 'amd64'
    
    def get_error_message(self, url):
        self.logger.debug("url is "+url)
        try:
            r = self.session.get(url=url)
            if (r.status_code != 200):
                self.logger.error("get launch error: {0}".format(r.text))
            self.logger.debug(json.dumps(r.json(), indent=4, sort_keys=True))
            if len(r.json()["content"]) == 0:
                self.logger.debug("output is empty")
            for ret in r.json()["content"]:
                if ret["level"] == "ERROR":
                    self.logger.debug(ret["message"])
                    return ret["message"]
            return ""
        except BaseException as e:
            print(e)
            return ""
        
    def check_system_issue(self, message):
        if "unable to handle the request" in message:
            return True
        if "connection refused" in message:
            return True
        if "dial tcp: lookup" in message and "no such host" in message:
            return True
        if "dial tcp" in message and "i/o timeout" in message:
            return True
        if "TLS handshake timeout" in message:
            return True
        if "server misbehaving" in message:
            return True
        if "the server was unable to return a response" in message:
            return True
        if "client connection lost" in message:
            return True
        if "request did not complete within requested timeout" in message:
            return True
        if "Unable to connect to the server" in message:
            return True
        return False


    def get_prow_case_result(self):
        self.logger.info("get_prow_case_result")
        day_number = self.days
        filter_url = self.launch_url + '?filter.has.compositeAttribute=version:{0}&filter.btw.startTime=-{1};1440;-0000&page.size=2000'.format(self.version, str(1440*day_number))
        self.logger.debug("filter_url is "+filter_url)
        try:
            r = self.session.get(url=filter_url)
            if (r.status_code != 200):
                self.logger.error("get launch error: {0}".format(r.text))
            self.logger.debug(json.dumps(r.json(), indent=4, sort_keys=True))
            if len(r.json()["content"]) == 0:
                self.logger.debug("no launch found")
            lanch_number = 0
            for ret in r.json()["content"]:
                if not ret["statistics"]["executions"]:
                    continue
                build_version = ''
                architecture = ''
                profilename = ''
                for attribute in ret['attributes']:
                    if attribute['key'] == 'version_installed':
                        build_version = attribute['value']
                    if attribute['key'] == 'architecture':
                        architecture = attribute['value']
                    if attribute['key'] == 'profilename':
                        profilename = attribute['value']
                platform = self.get_platform(profilename)
                start_time = ret["startTime"]
                id = ret["id"]
                date_str = datetime.fromtimestamp(int(start_time)/1000).strftime('%Y-%m-%d')
                link = self.ui_url+str(id)
                name = ret["name"]

                time.sleep(1)
                self.logger.info("get result from: %s: %s %s", lanch_number, name, id)
                lanch_number = lanch_number +1
                item_url = self.item_url + "?filter.eq.launchId={0}&launchesLimit=0&page.size=400&page.page=1".format(id)
                self.logger.debug(item_url)
                try:
                    launch_result = self.session.get(url=item_url)
                    if (launch_result.status_code != 200):
                        self.logger.error("get item case error: {0}".format(launch_result.text))
                    if len(launch_result.json()["content"]) == 0:
                        return ''
                    self.logger.debug(json.dumps(launch_result.json(), indent=4, sort_keys=True))
                    total_pages = launch_result.json()["page"]["totalPages"]
                    
                    for page_number in range(1, total_pages+1):
                        if page_number == 1:
                            content = launch_result.json()["content"]
                        else:
                            item_url_tmp = item_url.replace("page.page=1", "page.page="+str(page_number))
                            launch_result_tmp = self.session.get(url=item_url_tmp)
                            if (launch_result_tmp.status_code != 200):
                                self.logger.error("get item case error: {0}".format(launch_result_tmp.text))
                            if len(launch_result_tmp.json()["content"]) == 0:
                                break
                            self.logger.debug(json.dumps(launch_result_tmp.json(), indent=4, sort_keys=True))
                            content = launch_result_tmp.json()["content"]

                        for ret in content:
                            if ret["type"] == "STEP":
                                subteamOut = ret["pathNames"]["itemPaths"][0]["name"].replace("_cucushift", "")
                                name = ret["name"]
                                status = ret["status"]
                                caseitemid = ret["id"]
                                errorMsg = ""
                                system_issue = False
                                if status == "FAILED":
                                    log_url = self.base_url + "/api/v1/prow/log?filter.eq.item={0}".format(caseitemid)
                                    errorMsg = self.get_error_message(log_url)
                                    system_issue = self.check_system_issue(errorMsg)

                                caseids = re.findall(r'OCP-\d{4,}', name)
                                caseAuthor = ""
                                title = ""
                                if len(caseids) > 0:
                                    if ":" in name:
                                        caseAuthor = name.split(":")[1]
                                        title = name.split(":")[-1]
                                    else:
                                        caseAuthor = ""
                                        title = name
                                    for caseid in caseids:
                                        if caseid not in self.cases_result.keys():
                                            self.cases_result[caseid] = dict()
                                        self.cases_result[caseid]["subteam"] = subteamOut
                                        self.cases_result[caseid]["prow"+str(id)] = dict()
                                        self.cases_result[caseid]["prow"+str(id)]["status"] = status
                                        self.cases_result[caseid]["prow"+str(id)]["caseAuthor"] = caseAuthor
                                        self.cases_result[caseid]["prow"+str(id)]["link"] = link
                                        self.cases_result[caseid]["prow"+str(id)]["date"] = date_str
                                        self.cases_result[caseid]["prow"+str(id)]["buildversion"] = build_version
                                        self.cases_result[caseid]["prow"+str(id)]["architecture"] = architecture
                                        self.cases_result[caseid]["prow"+str(id)]["profilename"] = profilename
                                        self.cases_result[caseid]["prow"+str(id)]["platfrom"] = platform
                                        self.cases_result[caseid]["prow"+str(id)]["title"] = title
                                        self.cases_result[caseid]["prow"+str(id)]["errorMsg"] = errorMsg
                                        self.cases_result[caseid]["prow"+str(id)]["system_issue"] = system_issue
                                        
                                else:
                                    if name not in self.cases_result.keys():
                                        self.cases_result[name] = dict()
                                    self.cases_result[name]["subteam"] = subteamOut
                                    self.cases_result[name]["prow"+str(id)] = dict()
                                    self.cases_result[name]["prow"+str(id)]["caseAuthor"] = ""
                                    self.cases_result[name]["prow"+str(id)]["status"] = status 
                                    self.cases_result[name]["prow"+str(id)]["link"] = link
                                    self.cases_result[name]["prow"+str(id)]["date"] = date_str
                                    self.cases_result[name]["prow"+str(id)]["buildversion"] = build_version
                                    self.cases_result[name]["prow"+str(id)]["architecture"] = architecture
                                    self.cases_result[name]["prow"+str(id)]["profilename"] = profilename  
                                    self.cases_result[name]["prow"+str(id)]["platfrom"] = platform
                                    self.cases_result[name]["prow"+str(id)]["title"] = name
                                    self.cases_result[name]["prow"+str(id)]["errorMsg"] = errorMsg
                                    self.cases_result[name]["prow"+str(id)]["system_issue"] = system_issue  
                    self.logger.debug(json.dumps(self.cases_result, indent=4, sort_keys=True))
                except BaseException as e:
                    self.logger.error(e)

            self.logger.debug(self.cases_result.keys())
            return self.cases_result
        except BaseException as e:
            print(e)
            return dict()
    
    def get_jenkins_case_result(self):
        self.logger.info("get_jenkins_case_result")
        day_number = self.days
        filter_version="version:"+self.version.replace(".", "_")
        filter_team = ""
        filter_url_list = []
        filter_launchtype="launchtype:golang,pipeline_type:prereleasepipeline"
        if self.sub_team.lower() == "all":
            filter_url = self.ocp_launch_url + '?filter.has.compositeAttribute={0},{1}&filter.btw.startTime=-{2};1440;-0000&page.size=2000'.format(filter_version,filter_launchtype,str(1440*day_number))
            filter_url_list.append(filter_url)
        else:
            for sub_team_index in self.sub_team.split(","):
                filter_team="team:"+sub_team_index
                filter_url = self.ocp_launch_url + '?filter.has.compositeAttribute={0},{1},{2}&filter.btw.startTime=-{3};1440;-0000&page.size=2000'.format(filter_version,filter_team,filter_launchtype,str(1440*day_number))
                filter_url_list.append(filter_url)
            
        
        for filter_url in filter_url_list:
        
            self.logger.info("filter_url is "+filter_url)
            try:
                r = self.session.get(url=filter_url)
                if (r.status_code != 200):
                    self.logger.error("get launch error: {0}".format(r.text))
                self.logger.debug(json.dumps(r.json(), indent=4, sort_keys=True))
                if len(r.json()["content"]) == 0:
                    self.logger.debug("no launch found")
                lanch_number = 0
                for ret in r.json()["content"]:
                    if not ret["statistics"]["executions"]:
                        continue
                    build_version = ''
                    architecture = ''
                    profilename = ''
                    for attribute in ret['attributes']:
                        if attribute['key'] == 'build_version':
                            build_version = attribute['value']
                        if attribute['key'] == 'profilename':
                            profilename = attribute['value']
                    platform = self.get_jenkins_platform(profilename)
                    architecture = self.get_architecture(build_version)
                    start_time = ret["startTime"]
                    id = ret["id"]
                    date_str = datetime.fromtimestamp(int(start_time)/1000).strftime('%Y-%m-%d')
                    link = self.ocp_ui_url+str(id)
                    name = ret["name"]

                    self.logger.info("get result from: %s: %s %s", lanch_number, name, id)
                    lanch_number = lanch_number +1
                    item_url = self.ocp_item_url + "?filter.eq.launchId={0}&launchesLimit=0&page.size=400&page.page=1".format(id)
                    self.logger.debug(item_url)
                    try:
                        launch_result = self.session.get(url=item_url)
                        if (launch_result.status_code != 200):
                            self.logger.error("get item case error: {0}".format(launch_result.text))
                        if len(launch_result.json()["content"]) == 0:
                            return ''
                        self.logger.debug(json.dumps(launch_result.json(), indent=4, sort_keys=True))
                        total_pages = launch_result.json()["page"]["totalPages"]
                        
                        for page_number in range(1, total_pages+1):
                            if page_number == 1:
                                content = launch_result.json()["content"]
                            else:
                                item_url_tmp = item_url.replace("page.page=1", "page.page="+str(page_number))
                                launch_result_tmp = self.session.get(url=item_url_tmp)
                                if (launch_result_tmp.status_code != 200):
                                    self.logger.error("get item case error: {0}".format(launch_result_tmp.text))
                                if len(launch_result_tmp.json()["content"]) == 0:
                                    break
                                self.logger.debug(json.dumps(launch_result_tmp.json(), indent=4, sort_keys=True))
                                content = launch_result_tmp.json()["content"]

                            for ret in content:
                                if ret["type"] == "STEP":
                                    subteamOut = ret["pathNames"]["itemPaths"][0]["name"].replace("_cucushift", "")
                                    name = ret["name"]
                                    status = ret["status"]
                                    caseitemid = ret["id"]
                                    errorMsg = ""
                                    system_issue = False
                                    if status == "FAILED":
                                        log_url = self.base_url + "/api/v1/ocp/log?filter.eq.item={0}".format(caseitemid)
                                        errorMsg = self.get_error_message(log_url)
                                        system_issue = self.check_system_issue(errorMsg)
                                    caseids = re.findall(r'OCP-\d{4,}', name)
                                    caseAuthor = ""
                                    title = ""
                                    if len(caseids) > 0:
                                        if ":" in name:
                                            caseAuthor = name.split(":")[1]
                                            title = name.split(":")[-1]
                                        else:
                                            caseAuthor = ""
                                            title = name
                                        for caseid in caseids:
                                            if caseid not in self.cases_result.keys():
                                                self.cases_result[caseid] = dict()
                                            self.cases_result[caseid]["subteam"] = subteamOut
                                            self.cases_result[caseid]["ocp"+str(id)] = dict()
                                            self.cases_result[caseid]["ocp"+str(id)]["status"] = status
                                            self.cases_result[caseid]["ocp"+str(id)]["caseAuthor"] = caseAuthor
                                            self.cases_result[caseid]["ocp"+str(id)]["link"] = link
                                            self.cases_result[caseid]["ocp"+str(id)]["date"] = date_str
                                            self.cases_result[caseid]["ocp"+str(id)]["buildversion"] = build_version
                                            self.cases_result[caseid]["ocp"+str(id)]["architecture"] = architecture
                                            self.cases_result[caseid]["ocp"+str(id)]["profilename"] = profilename
                                            self.cases_result[caseid]["ocp"+str(id)]["platfrom"] = platform
                                            self.cases_result[caseid]["ocp"+str(id)]["title"] = title
                                            self.cases_result[caseid]["ocp"+str(id)]["errorMsg"] = errorMsg
                                            self.cases_result[caseid]["ocp"+str(id)]["system_issue"] = system_issue
                                    else:
                                        if name not in self.cases_result.keys():
                                            self.cases_result[name] = dict()
                                        self.cases_result[name]["subteam"] = subteamOut
                                        self.cases_result[name]["ocp"+str(id)] = dict()
                                        self.cases_result[name]["ocp"+str(id)]["caseAuthor"] = ""
                                        self.cases_result[name]["ocp"+str(id)]["status"] = status 
                                        self.cases_result[name]["ocp"+str(id)]["link"] = link
                                        self.cases_result[name]["ocp"+str(id)]["date"] = date_str
                                        self.cases_result[name]["ocp"+str(id)]["buildversion"] = build_version
                                        self.cases_result[name]["ocp"+str(id)]["architecture"] = architecture
                                        self.cases_result[name]["ocp"+str(id)]["profilename"] = profilename  
                                        self.cases_result[name]["ocp"+str(id)]["platfrom"] = platform
                                        self.cases_result[name]["ocp"+str(id)]["title"] = name 
                                        self.cases_result[name]["ocp"+str(id)]["errorMsg"] = errorMsg
                                        self.cases_result[name]["ocp"+str(id)]["system_issue"] = system_issue   
                        self.logger.debug(json.dumps(self.cases_result, indent=4, sort_keys=True))
                    except BaseException as e:
                        self.logger.error(e)

                self.logger.debug(self.cases_result.keys())
            except BaseException as e:
                print(e)
        return self.cases_result

    def update_summary(self, summary_sheet, version, sheet_name):
        if version == "4.16":
            column_number = 7
            column_str = "G"
        if version == "4.15":
            column_number = 9
            column_str = "I"
        elif version == "4.14":
            column_number = 11
            column_str = "K"
        elif version == "4.13":
            column_number = 13
            column_str = "M"
        else:
            return
        self.logger.debug("update_summary: version is %s", version)
        values = summary_sheet.col_values(column_number)
        index = len(values)+1
        self.logger.debug("update %s to be %s", column_str+str(index), sheet_name.replace(self.version+"-", ""))
        summary_sheet.update_acell(column_str+str(index),sheet_name.replace(self.version+"-", ""))
        
    def write_e2e_google_sheet(self):
        self.get_prow_case_result()
        self.get_jenkins_case_result()
        spreadsheet_target = self.gclient.open_by_url(self.target_file)
        template = spreadsheet_target.worksheet("template")
        sheetName = self.version+"-"+date.today().strftime("%Y%m%d")
        worksheetTitle_list =[sheet.title for sheet in spreadsheet_target.worksheets()]
        self.logger.info(worksheetTitle_list)
        if "Monthly CI Pass Ratio Trend" in worksheetTitle_list:
            worksheet_target = spreadsheet_target.duplicate_sheet(template.id,1)
            self.logger.info("update Monthly CI Pass Ratio Trend")
            summary = spreadsheet_target.worksheet("Monthly CI Pass Ratio Trend")
            self.update_summary(summary, self.version, sheetName)
        else:
            worksheet_target = spreadsheet_target.duplicate_sheet(template.id,0)
        worksheet_target.update_title(sheetName)
        
        sheet_update_content = []
        row = 32
        row_start = 33
        result_subteam_by_platfrom = dict()
        for platform_index in self.platfrom_list:
            result_subteam_by_platfrom[platform_index]=dict()
        for case_number in self.cases_result:
            subteam = self.cases_result[case_number]["subteam"]
            if self.sub_team.lower() != "all":
                if "," in self.sub_team:
                    sub_team_list = self.sub_team.split(",")
                    if subteam not in sub_team_list:
                        continue
                elif subteam != self.sub_team:
                    continue
            self.logger.info("check %s result", case_number)
            passed = 0
            failed = 0
            failed_jobs = []
            for id in self.cases_result[case_number]:
                if id == "subteam":
                    continue
                status = self.cases_result[case_number][id]["status"]
                author = self.cases_result[case_number][id]["caseAuthor"]
                case_name = self.cases_result[case_number][id]["title"]
                if status == "PASSED":
                    passed = passed +1
                elif status == "FAILED":
                    failed = failed +1
                    system_issue = ""
                    self.logger.info(self.cases_result[case_number][id])
                    if self.cases_result[case_number][id]["system_issue"]:
                        system_issue = ":systerm issue"
                    failed_jobs.append(self.cases_result[case_number][id]["profilename"]+": "+self.cases_result[case_number][id]["buildversion"]+": "+ self.cases_result[case_number][id]["link"]+system_issue)
                else:
                    continue
                
                #update result_subteam_by_platfrom
                platfrom = self.cases_result[case_number][id]["platfrom"]
                if not platfrom:
                    self.logger.error("the platform is empty for %s %s", self.cases_result[case_number][id]["profilename"], self.cases_result[case_number][id]["link"])
                    continue
                if subteam not in result_subteam_by_platfrom[platfrom].keys():
                    result_subteam_by_platfrom[platfrom][subteam] = dict()
                    result_subteam_by_platfrom[platfrom][subteam]["pass"] = 0
                    result_subteam_by_platfrom[platfrom][subteam]["failed"] = 0

                if status == "PASSED":
                    result_subteam_by_platfrom[platfrom][subteam]["pass"] = result_subteam_by_platfrom[platfrom][subteam]["pass"] + 1
                elif status == "FAILED":
                    result_subteam_by_platfrom[platfrom][subteam]["failed"] = result_subteam_by_platfrom[platfrom][subteam]["failed"] + 1
                else:
                    continue
            
            if failed == 0:
                if self.skip_no_failure_cases:
                    self.logger.debug("skip %s", case_number)
                    continue
                if passed == 0:
                    continue
            pass_ratio = float(passed)/(passed+failed)
            row = row +1
            case_output = [case_number, case_name, author, subteam, passed, failed, pass_ratio, os.linesep.join(failed_jobs)]
            sheet_update_content.append(case_output)
            
        if row >= row_start:
            worksheet_target.update('A'+str(row_start)+':H'+str(row), sheet_update_content)
        #update worksheet summary part
        
        subteams = worksheet_target.col_values(1)
        row_number = 0
        for subteam in subteams:
            row_number = row_number + 1
            if not subteam:
                continue
            if subteam == "Total":
                break
            content = []
            for platfrom_index in self.platfrom_list:
                if subteam in result_subteam_by_platfrom[platfrom_index].keys():
                    pass_number = result_subteam_by_platfrom[platfrom_index][subteam]["pass"]
                    failed_number = result_subteam_by_platfrom[platfrom_index][subteam]["failed"]
                    total_number = pass_number + failed_number
                    if total_number !=0:
                        content.extend([pass_number, failed_number])
                    else:
                        content.extend([0,0])
                else:
                    content.extend([0,0])
            self.logger.info('update M%s:AF%s to %s', row_number, row_number, str(content))
            worksheet_target.update('M'+str(row_number)+':AF'+str(row_number), [content], value_input_option="USER_ENTERED")
        if self.parent_jira_issue:
            self.create_sub_jira_task(worksheet_target)
    
    def create_sub_jira_task_all(self):
        spreadsheet_target = self.gclient.open_by_url(self.target_file)
        worksheet = spreadsheet_target.worksheet(self.sheet_name)
        self.create_sub_jira_task(worksheet)
        
    
    def create_sub_jira_task(self, worksheet):
        subtasks = self.jiraManager.get_subtask_list(self.parent_jira_issue)
        values_list_all = worksheet.get_all_values()
        for row in range(1, len(values_list_all)):
            values_list = values_list_all[row]
            row_number = row+1
            if len(values_list) < 7:
                continue
            caseid = values_list[0]
            case_title = values_list[1]
            author = values_list[2]
            if "OCP-" not in values_list[0]:
                continue
            if len(values_list) > 9 and values_list[9]:
                self.logger.info("subtask has been created %s for %s", values_list[10], values_list[0])
                continue
            history = values_list[7]
            history_no_system_issue = []
            for history_index in history.split(os.linesep):
                if ":systerm issue" not in history_index:
                    history_no_system_issue.append(history_index)

            pass_ratio = float(values_list[6].replace("%",""))
            if pass_ratio < self.pass_rate_threshold and history_no_system_issue:
                self.logger.info("pass ratio is %f", pass_ratio)
                comments = self.version+": pass ratio is "+str(pass_ratio)+"%"+os.linesep+os.linesep.join(history_no_system_issue)
                self.logger.info(history)
                self.logger.info(comments)
                jira_link = self.jiraManager.create_sub_task(self.author_map_file, self.parent_jira_issue, subtasks, caseid, case_title, author, comments)
                worksheet.update_acell('J'+str(row_number), "https://issues.redhat.com/browse/"+jira_link)
        
    def collectResult(self):
        self.logger.info("Collect CI result")
        self.write_e2e_google_sheet()
        
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
        #self.logger.info(json.dumps(issue.raw['fields'], indent=4, sort_keys=True))
        for issue in issue.fields.subtasks:
            issues[issue.key] = dict()
            issues[issue.key]["summary"] = issue.fields.summary
            issues[issue.key]["link"] = "https://issues.redhat.com/browse/"+issue.key
            
        self.logger.info(pprint.pformat(issues, indent=1))
        #self.logger.debug(json.dumps(issue.raw['fields'], indent=4, sort_keys=True))
        return issues
    
    def create_sub_task(self, author_map_file, parent_jira, subtasks, case_id, case_title, author, comments):
        if author_map_file and os.path.exists(author_map_file):
            with open(author_map_file, 'r') as outfile:
                auth_map = json.load(outfile)
                self.logger.debug("using author config file %s", author_map_file)
        else:
            self.logger.debug("using default author configuration")     
            auth_map = {"xzha": "rhn-support-xzha",
                        "jiazha": "rhn-support-jiazha",
                        "kuiwang": "rhn-support-kuiwang",
                        "bandrade":"bandrade@redhat.com",
                        "scolange": "rhn-support-xzha",
                        "tbuskey": "rhn-support-xzha",
                        "jitli": "rhn-support-jitli"
            }
        description_str = """
Hi, @{author}
{case} {title} is unstable, please help to check it.
{comments}
""".format(author=auth_map[author], case=case_id, title=case_title, comments=comments)
        for substask in subtasks.keys():
            if case_id.lower() in subtasks[substask]["summary"].lower():
                self.logger.info("add comments to %s", substask)
                self.jira.add_comment(substask, description_str)
                case_issue = self.jira.issue(substask)
                if case_issue.fields.status.name in ['Closed']:
                    self.jira.transition_issue(case_issue, transition='NEW')
                return substask 
        self.logger.info("Create sub task for %s", case_id)
        if not case_id:
            return
        parent_issue = self.jira.issue(parent_jira)
        project_key = parent_issue.fields.project.key
        parent_issue_key = parent_issue.key
      
        subtask = self.jira.create_issue(
                        project=project_key,
                        summary=case_id+' is unstable',
                        description=description_str,
                        issuetype={'name': 'Sub-task'},
                        parent={'key': parent_issue_key},
                        assignee= {"name": auth_map[author]}
        )

        self.logger.info("--------- Sub-task %s is created SUCCESS ----------", subtask.key)
        self.logger.debug(json.dumps(subtask.raw['fields'], indent=4, sort_keys=True))
        return subtask.key
       

########################################################################################################################################
if __name__ == "__main__":
    parser = argparse.ArgumentParser(prog="python3 collect_result.py", usage='''%(prog)s''')
    parser.add_argument("-t","--token", default="", required=False, help="the token of the RP")
    parser.add_argument("-k","--key", default="", required=False, help="the key file path")
    parser.add_argument("-f","--file", default="", required=False, help="the target google sheet file")
    parser.add_argument("-s", "--subteam", default="OLM", required=False, help="the sub team name")
    parser.add_argument("-log","--log", default="", required=False, help="the log file")
    parser.add_argument("-v", "--version", default='4.14', required=False, help="the ocp version")
    parser.add_argument("-d", "--days", default=7, type=int, required=False, help="the days number")
    parser.add_argument("-p", "--parent_jira", default="", required=False, help="the parent jira issue link")
    parser.add_argument("-jt", "--jira_token", default="", required=False, help="the jira token")
    parser.add_argument("--pass_rate_threshold", default=85, type=int, required=False, help="the pass rate threshold")
    parser.add_argument("--sheet", default="", required=False, help="the sheet link")
    parser.add_argument("--author", required=False, help="the map of the author, key is author in case, value is the jira id")
    parser.add_argument("--skip_no_failure_cases", dest='skip_no_failure_cases', default=False, required=False, action='store_true', help="skip cases whose pass ratio is 100%%")
    
    args=parser.parse_args()

    sclient = SummaryClient(args)
    #sclient.create_sub_jira_task_all()
    sclient.collectResult()
    #sclient.get_case_result("393167")
    
    exit(0)

    

    
