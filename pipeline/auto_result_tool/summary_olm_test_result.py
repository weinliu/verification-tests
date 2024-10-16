#!/usr/bin/python3
# author : xzha
import os
import argparse
import logging
import time
from urllib3.util import Retry
from datetime import date, datetime
import gspread
from oauth2client.service_account import ServiceAccountCredentials

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

        self.version = args.version

        self.key_file = args.key
        if not self.key_file:
            self.key_file = '/Users/zhaoxia/test/PROW/collect_result/key.json'
       
        self.gclient = self.getclient()
        self.target_file = args.file
        if not self.target_file:
            self.target_file = "https://docs.google.com/spreadsheets/d/1-SYjHKkttoYP0Nd1zgmGgonJIIuyiA-r6bpV77_KgkQ/edit?gid=0#gid=0"

    def getclient(self):
        scope = ['https://spreadsheets.google.com/feeds', 'https://www.googleapis.com/auth/drive']
        creds = ServiceAccountCredentials.from_json_keyfile_name(self.key_file, scope)
        return gspread.authorize(creds)
     
    def update_summary(self):
        spreadsheet_target = self.gclient.open_by_url(self.target_file)
        worksheet_summary = spreadsheet_target.worksheet("summary")
        values_list_release = worksheet_summary.row_values(1)
        values_list_url = worksheet_summary.row_values(2)
        release_dict = dict()
        index = 0
        for release_index in values_list_release:
            if release_index:
                release_dict[release_index] = values_list_url[index]
            index = index + 1
        self.logger.info(release_dict)
        test_result_summary = dict()
        for release in release_dict.keys():
            self.logger.info("get summary of release %s", release)
            if self.version and release not in self.version:
                continue
            test_result_summary[release] = dict()
            spreadsheet_release = self.gclient.open_by_url(release_dict[release])
            worksheet_list = spreadsheet_release.worksheets()
            time.sleep(2)
            self.logger.debug(release)
            for worksheet_index in worksheet_list:
                title = worksheet_index.title
                value =  worksheet_index.acell('L16').value
                if "template" not in worksheet_index.title:
                    test_result_summary[release][title.split("-")[-1]] = value
                time.sleep(2)
            self.logger.info(test_result_summary)
        for release_index in values_list_release:
            if not release_index:
                continue
            self.logger.info("update summary of release %s", release_index)
            if self.version and release_index not in self.version:
                continue
            update_content = []
            test_result_summary_release = sorted(test_result_summary[release_index])
            for date_index in test_result_summary_release:
                update_content.append([date_index, test_result_summary[release_index][date_index]])
            self.logger.info(release_index)
            self.logger.info(update_content)
            if "4.18" in release_index:
                worksheet_summary.update("G4:H"+str(3+len(test_result_summary_release)), update_content)
            if "4.17" in release_index:
                worksheet_summary.update("I4:J"+str(3+len(test_result_summary_release)), update_content)
            if "4.16" in release_index:
                worksheet_summary.update("K4:L"+str(3+len(test_result_summary_release)), update_content)
            if "4.15" in release_index:
                worksheet_summary.update("M4:N"+str(3+len(test_result_summary_release)), update_content)
            if "4.14" in release_index:
                worksheet_summary.update("O4:P"+str(3+len(test_result_summary_release)), update_content)
            if "4.13" in release_index:
                worksheet_summary.update("Q4:R"+str(3+len(test_result_summary_release)), update_content)
            if "4.12" in release_index:
                worksheet_summary.update("S4:T"+str(3+len(test_result_summary_release)), update_content)
                
                
                


########################################################################################################################################
if __name__ == "__main__":
    parser = argparse.ArgumentParser(prog="python3 collect_result.py", usage='''%(prog)s''')
    parser.add_argument("-k","--key", default="", required=False, help="the key file path")
    parser.add_argument("-f","--file", default="", required=False, help="the target google sheet file")
    parser.add_argument("-v","--version", default="", required=False, help="the version")
    parser.add_argument("-log","--log", default="", required=False, help="the log file") 
    args=parser.parse_args()

    sclient = SummaryClient(args)
    sclient.update_summary()
    
    exit(0)

    

    

