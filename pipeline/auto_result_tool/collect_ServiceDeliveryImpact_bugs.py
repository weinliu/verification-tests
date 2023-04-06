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
        
    def getIssues(self, filter=""):
        issues = dict()
        if not filter:
            filter = "labels = ServiceDeliveryImpact AND created >= startOfYear() ORDER BY Created DESC"
        issues_jira  = self.jira.search_issues(filter)
        for issue in issues_jira:
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
        self.logger.info(pprint.pformat(issues, indent=1))
        self.logger.debug(json.dumps(issue.raw['fields'], indent=4, sort_keys=True))
        return issues

class CollectClient:
    def __init__(self, args):
        self.logger = get_logger()
        self.token = args.token
        self.key = args.key
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
                             issues[bug_id]['status']]
            worksheet.insert_row(update_record, index=2)
            worksheet.update_acell('B2',f'=HYPERLINK("{link}","{summary}")')
            self.logger.info("================ update %s ======================", bug_id)
            time.sleep(5)
    
    def collectIssues(self):
        issues = self.jiraManager.getIssues()
        self.write_e2e_google_sheet(issues)
        
    def request_debug(self):
        spreadsheet = self.gclient.open_by_url(self.target_file)
        worksheet = spreadsheet.worksheet("ServiceDeliveryImpact Bugs")
        values_list_all = worksheet.get_all_values()
        message_list = []
        message_list.append('''For ServiceDeliveryImpact Bugs, https://docs.google.com/spreadsheets/d/1tU0IvHR9XahcBM_8kYZQXGIZiu79PG4X1X14XnZ1jeM/edit#gid=0, Please help to update the column H-L, Thanks!
Root Cause Analysed: when dev added comment or PR to fix the bug, QE needs to understand the root cause, which is helpful to design new test case to cover this scenario
Tested: QE already verify the fix with e2e scenario mentioned in the bug
Automated: the new test case is automated, if the case is manual only, please mark it as "Manual Only"\n''')
        for row in range(1, len(values_list_all)):
            values_list = values_list_all[row]
            bug_id = values_list[0]
            qa_contact = values_list[5]
            bug_status = values_list[6]
            rcaed = values_list[7]
            tested = values_list[8]
            automated = values_list[9]
            if bug_status.lower() == "verified" or bug_status.lower() == "closed":
                if not rcaed or not tested or not automated:
                    message_list.append(bug_id + ": @" +qa_contact.split(os.linesep)[0])
        self.logger.info(os.linesep.join(message_list))
            
            
        

########################################################################################################################################
if __name__ == "__main__":
    parser = argparse.ArgumentParser(prog="python3 collect_ServiceDeliveryImpact_bugs.py", usage='''%(prog)s''')
    parser.add_argument("-t","--token", required=True, help="the jira token")
    parser.add_argument("-k","--key", default="", required=False, help="the key file path")
    parser.add_argument("-r","--request_debug", dest='request_debug', default=False, action='store_true', help="the flag to request debug")
    args=parser.parse_args()

    cclient = CollectClient(args)
    if not args.request_debug:
        cclient.collectIssues()
    cclient.request_debug()
    exit(0)  
