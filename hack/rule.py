# encoding: utf-8
#!/usr/bin/env python3
import re, os

content = os.popen('git show master..', 'r').read()
print(content)

sigName = "sig-arch,sig-isc,sig-api-machinery,sig-auth,sig-apps,sig-cli,sig-scheduling,sig-etcd,sig-network,sig-network-edge,sig-storage,sig-openshift-logging,sig-devex, sig-builds,sig-ui,sig-instrumentation,sig-service-catalog,sig-operators,sig-imageregistry,sig-service-catalog,sig-hive,sig-windows,sig-testing,sig-scalability,sig-node,sig-node,sig-cluster-lifecycle,sig-node,sig-netobserv,sig-disasterrecovery"
sigNameList = sigName.replace(' ', '').split(",")

subTeam = "SDN,STORAGE,Developer_Experience,User_Interface,PerfScale,Service_Development_B,NODE,LOGGING,Logging,Apiserver_and_Auth,Workloads,Metering,Cluster_Observability,Quay/Quay.io,Cluster_Infrastructure,Multi-Cluster,Cluster_Operator,Azure,Network_Edge,ETCD,Installer,Portfolio_Integration,Service_Development_A,OLM,Operator_SDK,App_Migration,Windows_Containers,Security_and_Compliance,KNI,Openshift_Jenkins,RHV,ISV_Operators,PSAP,Multi-Cluster-Networking,OTA,Kata,Build_API,Image_Registry,Container_Engine_Tools,MCO,API_Server,Authentication,Hypershift,Network_Observability,DR_Testing"
subTeamList = subTeam.replace(' ', '').split(",")

importance = ["Critical","High","Medium","Low"]

patternDescribe = re.compile('\n\+.*g.Describe\(\"(\[(.*)\]\s(.*)\")')
patternIt = re.compile('\n\+\s+g.It\(\".*\"')

itContent = patternIt.findall(content)
desContent = patternDescribe.findall(content)

print("Des:", desContent)
print("it:", itContent)

findSig = False
findTeam = False

for des in desContent:
	# des[1] stores the sig name
	for sig in sigNameList:
		if sig in des[1]:
			findSig = True
			print("Description: %s, sigName: %s" % (des, sig))
			break
	# des[2] stores the sub team
	for team in subTeamList:
		if team in des[2]:
			findTeam = True
			print("Description: %s, teamNamee: %s" % (des, team))
			break
	
	if findSig and findTeam:
		print("PASS! sig name: %s | sub team: %s" % (des[1], des[2]))
	else:
		print("FAIL! sig or team name doesn't exist: %s" % (des[0],))
		raise Exception("please check the sig or team name above")

if len(itContent) > 0:
	mod = re.compile('g.It\((\"\D*)*(((?:Critical|High|Medium|Low)-\d+-)+)(.*)')
	for it in itContent:
		itMode = mod.findall(it)
		if len(itMode) > 0:
			for imp in importance:
				if imp.lower() in itMode[0][0].lower():
					print("FAIL! please follow the naming rule: %s" % (it))
					raise Exception("please check g.It title name above") 
			print("PASS! title name looks good!", it)
		else:
			print("FAIL! please follow the naming rule: %s" % (it))
			raise Exception("please check g.It title name above") 
