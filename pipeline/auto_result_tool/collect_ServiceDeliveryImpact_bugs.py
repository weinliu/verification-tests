import os
import json
import time
import argparse
import logging
import pprint
import gspread
from oauth2client.service_account import ServiceAccountCredentials
from jira import JIRA

def get_logger():       
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

class JIRAManager:
    def __init__(self, jira_server, token_auth, logger):
        self.logger = logger
        options = {
            'server': jira_server,
            'verify': True 
        }            
        self.jira = JIRA(options=options, token_auth=token_auth)
        
    def get_issues(self, filter=""):
        issues = dict()
        if not filter:
            filter = "labels in (ServiceDeliveryImpact, ServiceDeliveryBlocker) AND created >= startOfYear() ORDER BY Created DESC"
        issues_jira  = self.jira.search_issues(filter, maxResults=1000)
        for issue in issues_jira:
            if "OSDOCS" in issue.key:
                continue
            issues[issue.key] = dict()
            issues[issue.key]["summary"] = issue.fields.summary
            issues[issue.key]["link"] = "https://issues.redhat.com/browse/"+issue.key
            issues[issue.key]["created"] = issue.fields.created[0:10]
            try:
                issues[issue.key]["components"] = issue.fields.components[0].name
            except:
                issues[issue.key]["components"] = "unknown"
            try:
                issues[issue.key]["qe_contact_email"] = issue.fields.customfield_12315948.emailAddress
                issues[issue.key]["qe_contact_key"] = issue.fields.customfield_12315948.key
                issues[issue.key]["qe_contact_displayName"] = issue.fields.customfield_12315948.displayName
                issues[issue.key]["qe_contact"] = issue.fields.customfield_12315948.displayName + os.linesep + issue.fields.customfield_12315948.emailAddress
            except:
                issues[issue.key]["qe_contact"] = "unknown"
            try:
                issues[issue.key]["status"] = issue.fields.status.name
            except:
                issues[issue.key]["status"] = "unknown"
            try:
                issues[issue.key]["type"] = issue.fields.issuetype.name
            except:
                issues[issue.key]["type"] = "unknown"
            try:
                issues[issue.key]["labels"] = issue.fields.labels
            except:
                issues[issue.key]["labels"] = []
            try:
                issues[issue.key]["Target Version"] = issue.fields.customfield_12319940[0].name
            except:
                issues[issue.key]["Target Version"] = ""
            if len(issues[issue.key]["Target Version"]) == 0:
                try:
                    for fix_version in issue.fields.fixVersions:
                        issues[issue.key]["Target Version"] = issues[issue.key]["Target Version"] + fix_version.name
                except:
                    issues[issue.key]["Target Version"] = ""
            
        self.logger.debug(pprint.pformat(issues, indent=1))
        self.logger.debug(json.dumps(issue.raw['fields'], indent=4, sort_keys=True))
        return issues
    
    def create_sub_task(self, bug_id):
        self.logger.info("Create sub task for %s", bug_id)
        if not bug_id:
            return
        parent_issue = self.jira.issue('OCPQE-14652')
        project_key = parent_issue.fields.project.key
        parent_issue_key = parent_issue.key
        self.logger.debug(json.dumps(parent_issue.raw['fields'], indent=4, sort_keys=True))
        qe_contact = ""
        qe_displayname = ""
        try:
            bug_issue = self.jira.issue(bug_id)
            self.logger.debug(json.dumps(bug_issue.raw['fields'], indent=4, sort_keys=True))
        except Exception as e:
            self.logger.error("cannot get qe_contact for bug %s, %s", bug_id, e.text)
            return
        try:
            qe_contact = bug_issue.fields.customfield_12315948.name
            qe_displayname = bug_issue.fields.customfield_12315948.displayName
        except:
            qe_contact = "rhn-support-xzha"
            qe_displayname = ""
        description_str = """
Hi, {qe}
To support Objective 1, OKR 3 ServiceDeliveryImpacted ServiceDeliveryBlocker Bugs created since Jan 1, 2023 are RCAed, tested and automated, please make sure {bug} is RCAed, tested and automated.
Please update column I-M of bellow spreadsheet , thanks.
https://docs.google.com/spreadsheets/d/1tU0IvHR9XahcBM_8kYZQXGIZiu79PG4X1X14XnZ1jeM/edit#gid=0
""".format(qe=qe_displayname, bug=bug_id)
        subtask = self.jira.create_issue(
                        project=project_key,
                        summary=bug_id+' is RCAed, tested and automated',
                        description=description_str,
                        issuetype={'name': 'Sub-task'},
                        parent={'key': parent_issue_key},
                        assignee= {"name": qe_contact}
        )
        self.logger.info("--------- Sub-task %s is created SUCCESS ----------", subtask.key)
        self.logger.debug(json.dumps(subtask.raw['fields'], indent=4, sort_keys=True))
        return subtask.key
class CollectClient:
    def __init__(self, args):
        self.logger = get_logger()
        self.token = args.token
        self.key = args.key
        self.create_jira = args.create_jira
        self.target_file = 'https://docs.google.com/spreadsheets/d/1tU0IvHR9XahcBM_8kYZQXGIZiu79PG4X1X14XnZ1jeM/edit#gid=0'
        self.jiraManager = JIRAManager("https://issues.redhat.com", self.token, self.logger)
        
        scope = ['https://spreadsheets.google.com/feeds', 'https://www.googleapis.com/auth/drive']
        creds = ServiceAccountCredentials.from_json_keyfile_name(self.key, scope)
        self.gclient = gspread.authorize(creds)

    def write_e2e_google_sheet(self, issues):
        spreadsheet = self.gclient.open_by_url(self.target_file)
        worksheet = spreadsheet.worksheet("ServiceDeliveryImpact Bugs")
        values_list_all = worksheet.get_all_values()
        for row in range(1, len(values_list_all)):
            values_list = values_list_all[row]
            self.logger.debug("check row %s: %s", str(row+1), str(values_list))
            bug_id = values_list[0]
            if bug_id not in issues.keys():
                self.logger.info("%s: the label ServiceDeliveryImpact has been deleted", bug_id)
                continue
            if issues[bug_id]['components'] != values_list[3]:
                worksheet.update_acell("D"+str(row+1), issues[bug_id]['components'])
                self.logger.info("update D%s: %s", str(row+1), issues[bug_id]['components'])
                time.sleep(2)
            if issues[bug_id]['qe_contact'] != values_list[5]:
                worksheet.update_acell("F"+str(row+1), issues[bug_id]['qe_contact'])
                self.logger.info("update F%s: %s", str(row+1), issues[bug_id]['qe_contact'])
                time.sleep(2)
            if issues[bug_id]['status'] != values_list[6]:
                worksheet.update_acell("G"+str(row+1), issues[bug_id]['status'])
                self.logger.info("update G%s: %s", str(row+1), issues[bug_id]['status'])
                time.sleep(2)
            if issues[bug_id]['Target Version'] != values_list[7]:
                worksheet.update_acell("H"+str(row+1), issues[bug_id]['Target Version'])
                self.logger.info("update H%s: %s", str(row+1), issues[bug_id]['Target Version'])
                time.sleep(2)
            issues.pop(bug_id)
            
        for bug_id in issues.keys():
            self.logger.info("insert record with %s", str(issues[bug_id]))
            link = issues[bug_id]['link']
            summary = issues[bug_id]['summary']
            update_record = [bug_id,
                             '',
                             issues[bug_id]['type'],
                             issues[bug_id]['components'],
                             issues[bug_id]['created'],
                             issues[bug_id]['qe_contact'],
                             issues[bug_id]['status'],
                             issues[bug_id]['Target Version']]
            worksheet.insert_row(update_record, index=2)
            worksheet.update_acell('B2',f'=HYPERLINK("{link}","{summary}")')
            self.logger.info("================ update %s ======================", bug_id)
            time.sleep(5)
    
    def collect_issues(self):
        issues = self.jiraManager.get_issues()
        self.write_e2e_google_sheet(issues)
        
    def request_debug(self):
        spreadsheet = self.gclient.open_by_url(self.target_file)
        worksheet = spreadsheet.worksheet("ServiceDeliveryImpact Bugs")
        values_list_all = worksheet.get_all_values()
        message_list = []
        message_list.append('''For ServiceDeliveryImpact Bugs, https://docs.google.com/spreadsheets/d/1tU0IvHR9XahcBM_8kYZQXGIZiu79PG4X1X14XnZ1jeM/edit#gid=0, Please help to update the column I-M, Thanks!
Root Cause Analysed: when dev added comment or PR to fix the bug, QE needs to understand the root cause, which is helpful to design new test case to cover this scenario
Tested: QE already verify the fix with e2e scenario mentioned in the bug
Automated: the new test case is automated, if the case is manual only, please mark it as "Manual Only"\n''')
        for row in range(1, len(values_list_all)):
            values_list = values_list_all[row]
            bug_id = values_list[0]
            type = values_list[2]
            component = values_list[3]
            qa_contact = values_list[5]
            bug_status = values_list[6]
            rcaed = values_list[8]
            tested = values_list[9]
            automated = values_list[10]
            if "bug" not in type.lower():
                continue
            if bug_status.lower() == "verified" or bug_status.lower() == "closed" or bug_status.lower() == "on_qa":
                if not rcaed or not tested or not automated:
                    message_list.append(bug_id + " "+ component +" @" +qa_contact.split(os.linesep)[0])
        self.logger.info(os.linesep.join(message_list))
    
    def create_issue(self):
        if not self.create_jira:
            self.logger.warning("create jira is empty!")
            return
        spreadsheet = self.gclient.open_by_url(self.target_file)
        worksheet = spreadsheet.worksheet("ServiceDeliveryImpact Bugs")
        values_list_all = worksheet.get_all_values()
        for row in range(1, len(values_list_all)):
            values_list = values_list_all[row]
            bug_id = values_list[0]
            if bug_id == self.create_jira:
                subtask_id = self.jiraManager.create_sub_task(bug_id)
                if subtask_id:
                    worksheet.update_acell("N"+str(row+1), "https://issues.redhat.com/browse/"+subtask_id)
                else:
                    self.logger.error("create sub-task for %s failed", bug_id)
                return
        self.logger.error("There is no %s in worksheet", self.create_jira)
    

########################################################################################################################################
if __name__ == "__main__":
    parser = argparse.ArgumentParser(prog="python3 collect_ServiceDeliveryImpact_bugs.py", usage='''%(prog)s''')
    parser.add_argument("-t","--token", required=True, help="the jira token")
    parser.add_argument("-k","--key", default="", required=False, help="the key file path")
    parser.add_argument("-r","--request_debug", dest='request_debug', default=False, action='store_true', help="the flag to request debug")
    parser.add_argument("-c", "--create_jira", default="", required=False, help="create jira ticket")
    args=parser.parse_args()
        
    cclient = CollectClient(args)
    if args.create_jira:
       cclient.create_issue()
    else:
        if not args.request_debug:
            cclient.collect_issues()
        cclient.request_debug()
    exit(0)  
