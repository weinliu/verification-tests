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
import json
import matplotlib.pyplot as plt
import os
import pandas as pd
class GetDataFromRP:
    subteam = [
                "SDN","STORAGE","PerfScale","NODE","LOGGING","Logging","Workloads","Cluster_Observability","Cluster_Infrastructure",
                "Cluster_Operator","Network_Edge","ETCD","OLM","Operator_SDK","Windows_Containers","Security_and_Compliance",
                "PSAP","OTA","Image_Registry","Container_Engine_Tools","MCO","API_Server","Authentication","Network_Observability",
                "DR_Testing","OAP"
            ]
    teamColor = {
                "SDN": "#00008B",
                "STORAGE": "#008B8B",
                "PerfScale": "#B8860B",
                "NODE": "#FF00FF",
                "LOGGING": "#006400",
                "Logging": "#A9A9A9",
                "Workloads": "#BDB76B",
                "Cluster_Observability": "#8B008B",
                "Cluster_Infrastructure": "#556B2F",
                "Cluster_Operator": "#FF8C00",
                "Network_Edge": "#9932CC",
                "ETCD": "#8B0000",
                "OLM": "#E9967A",
                "Operator_SDK": "#8FBC8F",
                "Windows_Containers": "#483D8B",
                "Security_and_Compliance": "#2F4F4F",
                "PSAP": "#00CED1",
                "OTA": "#9400D3",
                "Image_Registry": "#EE82EE",
                "Container_Engine_Tools": "#FFA500",
                "MCO": "#008000",
                "API_Server": "#800080",
                "Authentication": "#D2691E",
                "Network_Observability": "#FF0000",
                "DR_Testing": "#000000",
                "OAP": "#FF1493",
    }

    # col = {'aliceblue': '#F0F8FF', 'antiquewhite': '#FAEBD7', 'aqua': '#00FFFF', 'aquamarine': '#7FFFD4', 
    #        'azure': '#F0FFFF', 'beige': '#F5F5DC', 'bisque': '#FFE4C4', 'black': '#000000', 
    #        'blanchedalmond': '#FFEBCD', 'blue': '#0000FF', 'blueviolet': '#8A2BE2', 'brown': '#A52A2A', 
    #        'burlywood': '#DEB887', 'cadetblue': '#5F9EA0', 'chartreuse': '#7FFF00', 'chocolate': '#D2691E', 
    #        'coral': '#FF7F50', 'cornflowerblue': '#6495ED', 'cornsilk': '#FFF8DC', 'crimson': '#DC143C', 
    #        'cyan': '#00FFFF', 'darkblue': '#00008B', 'darkcyan': '#008B8B', 'darkgoldenrod': '#B8860B', 
    #        'darkgray': '#A9A9A9', 'darkgreen': '#006400', 'darkgrey': '#A9A9A9', 'darkkhaki': '#BDB76B', 
    #        'darkmagenta': '#8B008B', 'darkolivegreen': '#556B2F', 'darkorange': '#FF8C00', 
    #        'darkorchid': '#9932CC', 'darkred': '#8B0000', 'darksalmon': '#E9967A', 'darkseagreen': '#8FBC8F', 
    #        'darkslateblue': '#483D8B', 'darkslategray': '#2F4F4F', 'darkslategrey': '#2F4F4F', 
    #        'darkturquoise': '#00CED1', 'darkviolet': '#9400D3', 'deeppink': '#FF1493', 'deepskyblue': '#00BFFF', 
    #        'dimgray': '#696969', 'dimgrey': '#696969', 'dodgerblue': '#1E90FF', 'firebrick': '#B22222', 
    #        'floralwhite': '#FFFAF0', 'forestgreen': '#228B22', 'fuchsia': '#FF00FF', 'gainsboro': '#DCDCDC', 
    #        'ghostwhite': '#F8F8FF', 'gold': '#FFD700', 'goldenrod': '#DAA520', 'gray': '#808080', 
    #        'green': '#008000', 'greenyellow': '#ADFF2F', 'grey': '#808080', 'honeydew': '#F0FFF0', 
    #        'hotpink': '#FF69B4', 'indianred': '#CD5C5C', 'indigo': '#4B0082', 'ivory': '#FFFFF0', 
    #        'khaki': '#F0E68C', 'lavender': '#E6E6FA', 'lavenderblush': '#FFF0F5', 'lawngreen': '#7CFC00', 
    #        'lemonchiffon': '#FFFACD', 'lightblue': '#ADD8E6', 'lightcoral': '#F08080', 'lightcyan': '#E0FFFF', 
    #        'lightgoldenrodyellow': '#FAFAD2', 'lightgray': '#D3D3D3', 'lightgreen': '#90EE90', 
    #        'lightgrey': '#D3D3D3', 'lightpink': '#FFB6C1', 'lightsalmon': '#FFA07A', 'lightseagreen': '#20B2AA', 
    #        'lightskyblue': '#87CEFA', 'lightslategray': '#778899', 'lightslategrey': '#778899', 
    #        'lightsteelblue': '#B0C4DE', 'lightyellow': '#FFFFE0', 'lime': '#00FF00', 'limegreen': '#32CD32', 
    #        'linen': '#FAF0E6', 'magenta': '#FF00FF', 'maroon': '#800000', 'mediumaquamarine': '#66CDAA', 
    #        'mediumblue': '#0000CD', 'mediumorchid': '#BA55D3', 'mediumpurple': '#9370DB', 
    #        'mediumseagreen': '#3CB371', 'mediumslateblue': '#7B68EE', 'mediumspringgreen': '#00FA9A', 
    #        'mediumturquoise': '#48D1CC', 'mediumvioletred': '#C71585', 'midnightblue': '#191970', 
    #        'mintcream': '#F5FFFA', 'mistyrose': '#FFE4E1', 'moccasin': '#FFE4B5', 'navajowhite': '#FFDEAD', 
    #        'navy': '#000080', 'oldlace': '#FDF5E6', 'olive': '#808000', 'olivedrab': '#6B8E23', 
    #        'orange': '#FFA500', 'orangered': '#FF4500', 'orchid': '#DA70D6', 'palegoldenrod': '#EEE8AA', 
    #        'palegreen': '#98FB98', 'paleturquoise': '#AFEEEE', 'palevioletred': '#DB7093', 
    #        'papayawhip': '#FFEFD5', 'peachpuff': '#FFDAB9', 'peru': '#CD853F', 'pink': '#FFC0CB', 
    #        'plum': '#DDA0DD', 'powderblue': '#B0E0E6', 'purple': '#800080', 'rebeccapurple': '#663399', 
    #        'red': '#FF0000', 'rosybrown': '#BC8F8F', 'royalblue': '#4169E1', 'saddlebrown': '#8B4513', 
    #        'salmon': '#FA8072', 'sandybrown': '#F4A460', 'seagreen': '#2E8B57', 'seashell': '#FFF5EE', 
    #        'sienna': '#A0522D', 'silver': '#C0C0C0', 'skyblue': '#87CEEB', 'slateblue': '#6A5ACD', 
    #        'slategray': '#708090', 'slategrey': '#708090', 'snow': '#FFFAFA', 'springgreen': '#00FF7F', 
    #        'steelblue': '#4682B4', 'tan': '#D2B48C', 'teal': '#008080', 'thistle': '#D8BFD8', 'tomato': 
    #        '#FF6347', 'turquoise': '#40E0D0', 'violet': '#EE82EE', 'wheat': '#F5DEB3', 'white': '#FFFFFF', 
    #        'whitesmoke': '#F5F5F5', 'yellow': '#FFFF00', 'yellowgreen': '#9ACD32'}

    def __init__(self, args):
        if args.token == "":
            with open("./prowrp.key") as f:
                token_f = yaml.safe_load(f)
                args.token = token_f["prow_mmtoken"]
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
        self.item_url = args.endpoint + "/v1/" + args.project + "/item/v2"
        self.args = args


    def makeLaunchFilterUrl(self, filters=None):
        filter_url = self.launch_url + "?page.page=1&page.size=300"+"&page.sort="+urllib.parse.quote("startTime,number,DESC")
        if filters["platform"] != "":
            filter_url = filter_url + "&filter.cnt.name=" + filters["platform"].replace(" ", "")

        if filters["release"] != "":
            release_key = "version:"+filters["release"]
            filter_url = filter_url + "&filter.has.compositeAttribute=" + urllib.parse.quote(release_key.replace(" ", ""))

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

        # print("LaunchFilterURL: {0}".format(filter_url))
        return filter_url


    def getLaunchInfoFromRsp(self, rsp):
        ids = []
        for ret in rsp:
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
        filter_url = self.item_url + "?filter.level.path=1&page.page=1&page.size=300"+"&page.sort="+urllib.parse.quote("startTime,ASC")

        if filters["launch"] != None:
            filter_url = filter_url + "&providerType=launch&launchId=" + str(filters["launch"]["id"])

        # print("ItemFilterURL: {0}".format(filter_url))
        return filter_url

    def getItemInfoFromRsp(self, rsp, filters):
        ids = []

        if len(rsp) == 0:
            print("no case match")
            return ids
        for ret in rsp:
            ids.append({"id":ret["id"], "name":ret["name"], "statistics":ret["statistics"]})

        return ids

    def getItems(self, filters):
        query_item_url = self.makeItemFilterUrl(filters)
        ids = []
        total_pages = 0

        r = self.session.get(url=query_item_url)
        if (r.status_code != 200):
            print("can not get suite status")
            return ids
        total_pages = r.json()["page"]["totalPages"]
        ids.extend(self.getItemInfoFromRsp(r.json()["content"], filters))

        for page_number in range(2, total_pages+1):
            query_item_url_tmp = query_item_url.replace("page.page=1", "page.page="+str(page_number))
            r = self.session.get(url=query_item_url_tmp)
            if (r.status_code != 200):
                print("can not get case ")
                print("continue next page, and please rerun it to try failed page")
                continue
            ids.extend(self.getItemInfoFromRsp(r.json()["content"], filters))

        return ids

    def getItemPerLaunch(self, launch):
        filters = {
            "launch": launch
        }
        items = self.getItems(filters)
        for item in items:
            pass
        return items
            # print(item)
            # ret_code = self.updateItemStatus(item["id"], "passed")
            # if (ret_code != 200) and (ret_code != 201):
            #     print("can not change status for case={0} in launch {1} #{2}. please rerun or manually change status".format(item["name"], launch["name"], launch["number"]))

    def GetTestData(self):
        if self.args.release == "" or self.args.platform == "" or self.args.timeduration == "":
            print("release, or platform or timeduration is not set")
            return
        filters = {
            "platform": self.args.platform,
            "timeDuration": self.args.timeduration,
            "subTeam": self.args.subteam,
            "release": self.args.release        }
        existinglaunchs = self.getLaunches(filters)
        if existinglaunchs == None:
            print("no launch match")
            return
        # print("we found launches:\n {0}".format(existinglaunchs))
        suites =  []
        for launch in existinglaunchs:
            # print("{0}:{1}".format(launch["id"], launch["name"]))
            suites.extend(self.getItemPerLaunch(launch))

        sumMods = {}
        for suite in suites:
            name = suite["name"]
            if "_cucushift" in name:
                continue
            if name not in self.subteam:
                continue
            passed = 0
            failed = 0
            if suite["statistics"]["executions"].get("passed") is not None:
                passed = suite["statistics"]["executions"]["passed"]
            if suite["statistics"]["executions"].get("failed") is not None:
                failed = suite["statistics"]["executions"]["failed"]
            # print(name, passed, failed)
            
            sumMod = sumMods.get(name)
            if sumMod is not None:
                sumMod["passed"] = sumMod["passed"] + passed
                sumMod["failed"] = sumMod["failed"] + failed
            else:
                sumMods[name] = {"passed": passed, "failed": failed}
        # print(sumMods)

        passrate = {}
        for mod, data in sumMods.items():
            if data["passed"]+data["failed"] == 0:
                continue
            r = int(data["passed"]/(data["passed"]+data["failed"]) * 100)
            passrate[mod]=r
        # check if there is no subteam not run which impact the curve
        for team in self.subteam:
            if team not in passrate:
                passrate[team]=-1

        morerp = {}
        st = datetime.strptime(self.args.timeduration.split(",")[0], "%Y-%m-%d %H:%M:%S").strftime('%Y-%m-%d-%H:%M')
        morerp[self.args.platform] = {st: passrate}

        rpjsonfile = "./rpstat-"+self.args.release+".json"
        if os.path.exists(rpjsonfile):
            with open(rpjsonfile, 'r') as f:
                dataExisting = json.load(f)
        else:
            with open(rpjsonfile, "w") as fp:
                json.dump(morerp, fp, sort_keys=True, indent=2)
            return

        dp = dataExisting.get(self.args.platform)
        if  dp is None:
            dataExisting[self.args.platform] = {st: passrate}
        else:
            dp[st] = passrate

        print(json.dumps(dataExisting, sort_keys=True, indent=2))
        with open(rpjsonfile, "w") as fp:
            json.dump(dataExisting, fp, sort_keys=True, indent=2)


    def DrawPillarPerTeam(self):
        if self.args.release == "" or self.args.datafile == "":
            print("release or datafile is not set")
            return
        with open(self.args.datafile, 'r') as f:
            data = json.load(f)
        print(data.keys())
        for profile in data.keys():
            dataSumTimeSlot = data[profile]
            # print(dataSumTimeSlot)
            for timeslot in dataSumTimeSlot.keys():
                title = profile + " on " + timeslot
                sampleData = data[profile][timeslot]
                # print(sampleData)
                passrate = []
                subteam = []
                for team, rate in sampleData.items():
                    passrate.append(rate)
                    subteam.append(team)
                # print(passrate)
                # print(subteam)
                self.drawPillar(subteam, passrate, title, profile, timeslot, self.args.release)

    def drawPillar(self, subteam, passrate, title, profile, timeslot, release):
        plt.figure(figsize=(20, 8))
        x = range(len(subteam))
        bar_width = 0.2
        opacity = 0.8
        labels = tuple(subteam)
        xticks = [a + bar_width / 2 for a in x]
        for i, score in enumerate(passrate):
            plt.bar(x[i], score, bar_width, alpha=opacity, color=self.teamColor[subteam[i]])
            plt.text(x[i], score + 1, str(score), ha='center', fontsize=6, color='black')
        # plt.bar(subteam, passrate, bar_width, alpha=opacity)
        plt.xticks(xticks, labels, rotation=90)
        
        plt.xlabel("subteam")
        plt.ylabel("Passrate (%)")
        # plt.subplots_adjust(top=0.9)
        plt.title(title,loc='left',rotation=3)
        # plt.legend()
        plt.ylim(30, 100)
        plt.savefig("/tmp/"+"pillar-"+release+"--"+profile+"--"+timeslot.replace(":", "-")+".png")
        plt.show()

    def DrawCurvePerProfile(self):
        if self.args.release == "" or self.args.datafile == "":
            print("release or datafile is not set")
            return
        with open(self.args.datafile, 'r') as f:
            data = json.load(f)
        print(data.keys())
        for profile in data.keys():
            dataSumTimeSlot = data[profile]
            # print(dataSumTimeSlot)
            timeslots = sorted(list(dataSumTimeSlot.keys()))
            passrate = {}
            for timeslot in timeslots:
                stat = data[profile][timeslot]
                # print(stat)
                for t, p in stat.items():
                    if self.args.subteam != "" and t not in self.args.subteam:
                        continue 
                    # print(t, p)
                    # if p == -1:
                    #     p = 100
                    pr = passrate.get(t)
                    if pr is not None:
                        pr = pr.append(p)
                    else:
                        passrate[t] = [p]
            # print(passrate)
            self.drawCurve(passrate, timeslots, profile, self.args.release)

    def drawCurve(self, passrate, timeslots, profile, release):
        plt.figure(figsize=(20, 8))
        for team in passrate.keys():
            plt.plot(timeslots, passrate[team], color=self.teamColor[team], label=team)
            # plt.text(timeslots[-1], passrate[team][-1], team)
        plt.title(profile,loc='left',rotation=3)
        plt.xlabel("Time")
        plt.ylabel("Passrate (%)")
        plt.legend(loc='center left', bbox_to_anchor=(1, 0.5))
        plt.ylim(0, 100)
        plt.savefig("/tmp/"+"curve-"+release+"--"+profile+"--"+timeslots[0].replace(":", "-")+".png")
        plt.show()

    def GenerateXLS(self):
        if self.args.datafile == "" or self.args.outfile == "":
            print("datafile or outfile is not set")
            return
        if ".xlsx" not in self.args.outfile:
            print("please use .xlsx format")
            return
        with open(self.args.datafile, 'r') as f:
            data = pd.read_json(f)

        # convert json to dataframe
        sheets = []
        for sheet_name, sheet_data in data.items():
            sheet_dict = {}
            # cols = []
            for column_name, column_data in sheet_data.items():
                column_dict = {}
                rows = []
                # cols.append(column_name)
                for row_name, value in column_data.items():
                    column_dict[row_name] = value
                    rows.append(row_name)
                sheet_dict[column_name] = column_dict
            df = pd.DataFrame(sheet_dict, index=rows)
            sheets.append((sheet_name, df))

            # define writer
            writer = pd.ExcelWriter('data.xlsx', engine='xlsxwriter')

            # write to differnt worksheet with setting width
            for sheet in sheets:
                sheet_name, df = sheet
                df.to_excel(writer, sheet_name=sheet_name)
                worksheet = writer.sheets[sheet_name]
                for i, col in enumerate(df.columns):
                    width = max(df[col].astype(str).map(len).max(), len(col.strftime('%Y-%m-%d %H:%M:%S'))) + 1
                    worksheet.set_column(i, i, width)
                worksheet.set_column(i+1, i+1, width)

            # close file
            writer.save()

if __name__ == "__main__":
    parser = argparse.ArgumentParser("passrate.py")
    parser.add_argument("-a","--action", default="get", choices=["get", "pillar", "curve", "xls"], required=True)
    parser.add_argument("-e","--endpoint", default="https://reportportal-openshift.apps.dno.ocp-hub.prod.psi.redhat.com/api")
    parser.add_argument("-t","--token", default="")
    parser.add_argument("-p","--project", default="prow")

    parser.add_argument("-pt","--platform", default="")
    parser.add_argument("-s","--subteam", default="")
    parser.add_argument("-r","--release", default="")
    parser.add_argument("-td","--timeduration", default="")
    parser.add_argument("-f","--datafile", default="./rptestsum.json")
    parser.add_argument("-o","--outfile", default="./data.xlsx")
    args=parser.parse_args()

    gdfr = GetDataFromRP(args)
    if args.action == "get":
        gdfr.GetTestData()
    if args.action == "pillar":
        gdfr.DrawPillarPerTeam()
    if args.action == "curve":
        gdfr.DrawCurvePerProfile()
    if args.action == "xls":
        gdfr.GenerateXLS()
    exit(0)

