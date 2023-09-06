#!/usr/bin/python3
# author : xzha
import os
import re
import urllib3
import requests
import argparse
import json
import logging
from urllib3.exceptions import InsecureRequestWarning
from requests.adapters import HTTPAdapter
from urllib3.util import Retry
from datetime import date, datetime
import gspread
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
        if self.sub_team.lower() != "all":
            filter_team="team:"+self.sub_team
        filter_launchtype="launchtype:golang,pipeline_type:prereleasepipeline"
        if filter_team:
            filter_url = self.ocp_launch_url + '?filter.has.compositeAttribute={0},{1},{2}&filter.btw.startTime=-{3};1440;-0000&page.size=2000'.format(filter_version,filter_team,filter_launchtype,str(1440*day_number))
        else:
            filter_url = self.ocp_launch_url + '?filter.has.compositeAttribute={0},{1}&filter.btw.startTime=-{2};1440;-0000&page.size=2000'.format(filter_version,filter_launchtype,str(1440*day_number))

        
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
                    self.logger.debug(json.dumps(self.cases_result, indent=4, sort_keys=True))
                except BaseException as e:
                    self.logger.error(e)

            self.logger.debug(self.cases_result.keys())
            return self.cases_result
        except BaseException as e:
            print(e)
            return dict()
        

    def write_e2e_google_sheet(self):
        self.get_prow_case_result()
        self.get_jenkins_case_result()
        spreadsheet_target = self.gclient.open_by_url(self.target_file)
        template = spreadsheet_target.worksheet("template")
        worksheet_target = spreadsheet_target.duplicate_sheet(template.id)
        worksheet_target.update_title(self.version+"-"+date.today().strftime("%Y%m%d"))
        sheet_update_content = []
        row = 32
        row_start = 33
        result_subteam_by_platfrom = dict()
        for platform_index in self.platfrom_list:
            result_subteam_by_platfrom[platform_index]=dict()
        for case_number in self.cases_result:
            subteam = self.cases_result[case_number]["subteam"]
            if self.sub_team.lower() != "all":
                if subteam != self.sub_team:
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
                    failed_jobs.append(self.cases_result[case_number][id]["profilename"]+": "+self.cases_result[case_number][id]["buildversion"]+": "+ self.cases_result[case_number][id]["link"])
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
                self.logger.debug("skip %s", case_number)
                continue
            else:
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
        
         
            
    def collectResult(self):
        self.logger.info("Collect CI result")
        self.write_e2e_google_sheet()
        

########################################################################################################################################
if __name__ == "__main__":
    parser = argparse.ArgumentParser(prog="python3 collect_result.py", usage='''%(prog)s''')
    parser.add_argument("-t","--token", default="")
    parser.add_argument("-k","--key", default="", required=False, help="the key file path")
    parser.add_argument("-f","--file", default="", required=False, help="the target google sheet file")
    parser.add_argument("-s", "--subteam", default="OLM", required=False, help="the sub team name")
    parser.add_argument("-log","--log", default="", required=False, help="the log file")
    parser.add_argument("-v", "--version", default='4.14', help="the ocp version")
    parser.add_argument("-d", "--days", default=7, type=int, help="the days number")
    args=parser.parse_args()

    sclient = SummaryClient(args)
    sclient.collectResult()
    #sclient.get_case_result("393167")
    
    exit(0)

    

    
