#!/bin/bash
set -e
env|grep -v -E "GLOKIPWD"

function run {
  PIPELINESCRIPT_DIR=${WORKSPACE}"/private/pipeline" && export PATH=${PIPELINESCRIPT_DIR}:$PATH
  if [ ${REPO_OWNER} == "openshift" ]; then
    WORKBUILDDIR=${WORKSPACE}"/private"
  else
    eval mkdir -p ${WORKSPACE}"/public"
    WORKBUILDDIR=${WORKSPACE}"/public"
  fi
  cd ${WORKBUILDDIR}

  put_fake_launch_for_each_profile
  config_env
  id
  date
  select_fail_case_for_official_rerun
  execute
  result_report
}

function config_env {
  if (echo ${NODE_LABELS} | grep -E 'preserve-gvm'); then
    echo "config env for vm"
    config_env_for_vm
  else
    echo "config env for cluster"
    config_env_for_cluster
  fi
  go version
  go env
  env|grep -v -E "GLOKIPWD|GITHUB_TOKEN|GITHUB_API_CURL_OPTS"
  oc version -o yaml || echo "can not get oc version"
}

function config_env_for_vm {
  echo 'unset http_proxy https_proxy no_proxy'
  unset http_proxy https_proxy
  echo "home path is "${HOME}
  GOINSTALLPATH="/usr/local/go1.22.5/go"
  if [ ! -d "/usr/local/go1.22.5/go/bin" ]; then
    sudo mkdir /usr/local/go1.22.5
    sudo wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
    sudo tar -C /usr/local/go1.22.5/ -xzf go1.22.5.linux-amd64.tar.gz
  fi
  mkdir -p ${HOME}/kubeconf && mkdir -p ${HOME}/azureauth && \
  echo "export KUBECONFIG=${HOME}/kubeconf/kubeconfig" > ${WORKSPACE}/.bash_profile && \
  echo "export AZURE_AUTH_LOCATION=${HOME}/azureauth/azure_auth.json" >> ${WORKSPACE}/.bash_profile && \
  echo 'export GOROOT=${GOINSTALLPATH}' >> ${WORKSPACE}/.bash_profile && \
  echo 'export GOPATH=${WORKSPACE}/goproject' >> ${WORKSPACE}/.bash_profile && \
  echo 'export GOCACHE=${WORKSPACE}/gocache' >> ${WORKSPACE}/.bash_profile && \
  echo 'export PATH=$PATH:${GOINSTALLPATH}/bin:${WORKSPACE}/tool_tmp:/opt/OpenShift4-tools' >> ${WORKSPACE}/.bash_profile && \
  echo 'export PATH=${GOINSTALLPATH}/bin:$PATH:${WORKSPACE}/tool_tmp' >> ${WORKSPACE}/.bash_profile && \
  source ${WORKSPACE}/.bash_profile
  echo 'unset http_proxy https_proxy no_proxy'
  unset http_proxy https_proxy
  if [[ "x${http_proxy}x" != "xx" ]] || [[ "x${https_proxy}x" != "xx" ]]; then
    echo 'unset http_proxy https_proxy failed'
    exit 1
  fi
  echo "configure kubeconfig, azure authentication or client proxy for the cluster"
  source ${PIPELINESCRIPT_DIR}"/occe4c" ${WORKSPACE} "null"${FLEXY_BUILD} "${CONFIG}"
  echo "configure vm"
  ${PIPELINESCRIPT_DIR}"/setup-vm" ${JENKINS_AGENT} ${WORKBUILDDIR} ${WORKSPACE}"/tool_tmp"
}

function config_env_for_cluster {
  mkdir -p /home/jenkins/kubeconf && mkdir -p /home/jenkins/azureauth && \
  echo "export KUBECONFIG=/home/jenkins/kubeconf/kubeconfig" >> ~/.bash_profile && \
  echo "export AZURE_AUTH_LOCATION=/home/jenkins/azureauth/azure_auth.json" >> ~/.bash_profile && \
  echo 'export GOROOT=/usr/local/go' >> ~/.bash_profile && \
  echo 'export GOPATH=/goproject' >> ~/.bash_profile && \
  echo 'export GOCACHE=/gocache' >> ~/.bash_profile && \
  echo 'export PATH=$PATH:/usr/local/go/bin:/opt/OpenShift4-tools' >> ~/.bash_profile && \
  source ~/.bash_profile
  echo "configure kubeconfig, azure authentication or client proxy for the cluster"
  source ${PIPELINESCRIPT_DIR}"/occe4c" ${WORKSPACE} "null"${FLEXY_BUILD} "${CONFIG}"
  echo "opm version: "
  opm version
}
function result_report {
  echo "get result and parse it"
  LAUNCHTRIAL="yes"
  if [ "${TIERN_REPO_OWNER}" == "kuiwang02" ]; then
    ocgr ${WORKBUILDDIR} ${WORKSPACE} ${JENKINS_AGENT} "null"${LAUNCH_NAME} "null""${PROFILE_NAME}" "null""${LAUNCHTRIAL}" "openshift-""${REPO_OWNER}"             ${BUILD_NUMBER} "null""${FILTERS}" "null""${ADDITIONAL_ATTRIBUTES}"
  else
    ocgr ${WORKBUILDDIR} ${WORKSPACE} ${JENKINS_AGENT} "null"${LAUNCH_NAME} "null""${PROFILE_NAME}" "null""${LAUNCHTRIAL}" "${TIERN_REPO_OWNER}""-""${REPO_OWNER}" ${BUILD_NUMBER} "null""${FILTERS}" "null""${ADDITIONAL_ATTRIBUTES}"
  fi
}

function execute {
  echo "the scenario is \"${SCENARIO}\", and the importance is \"${IMPORTANCE}\""
  eval rm -fr ${WORKSPACE}"/private/junit_e2e_*.xml" ${WORKSPACE}"/public/junit_e2e_*.xml"
  cd ${WORKBUILDDIR}

  case "$REPO_OWNER" in
    openshift)
      echo "run case with oropenshift-tests-private under openshift or your account. similar to ocrd"
      echo "ocr ${TIERN_REPO_OWNER} \"${SCENARIO}\" ${IMPORTANCE} \"null${FILTERS}\"  \"null${CASE_TIMEOUT}\""
      ocr ${TIERN_REPO_OWNER} "${SCENARIO}" ${IMPORTANCE} "null${FILTERS}" "null${CASE_TIMEOUT}" || true
      ;;
    *)
      echo "run case with oropenshift-tests under your account. similar to ocru"
      echo "ocr null \"${SCENARIO}\" ${IMPORTANCE} \"null${FILTERS}\"  \"null${CASE_TIMEOUT}\""
      ocr "null" "${SCENARIO}" ${IMPORTANCE} "null${FILTERS}"  "null${CASE_TIMEOUT}"|| true
      ;;
  esac
}

function select_fail_case_for_official_rerun {
  echo "cause by rebuild is ${IS_REBUILD_CAUSE}"
  echo "cause by upstream is ${IS_UPSTREAM_CAUSE}"
  if ((echo ${LAUNCH_NAME} | grep -E '^([0-9]{8})-([0-9]{4})$') || \
      (echo ${LAUNCH_NAME} | grep -E '^([0-9]{8})-([0-9]{4})_([0-9]{1,2})$')) && \
      ([[ "${IS_REBUILD_CAUSE}" == "yes" ]] && [[ "${IS_UPSTREAM_CAUSE}" == "yes" ]]) && \
      ([[ "${TIERN_REPO_OWNER}" == "openshift" ]] || [[ "${TIERN_REPO_OWNER}" == "kuiwang02" ]]) && [[ "${REPO_OWNER}" == "openshift" ]]; then
    echo "valid launch name with reran pipeline build. Try to find fail case and update SCENARIO"
    failcaseid=`ocgfc ${WORKBUILDDIR} ${WORKSPACE} ${LAUNCH_NAME} "${SCENARIO}" ${BUILD_NUMBER} "null""${FILTERS}" 2>&1 || true`
    echo -e "${failcaseid}"
    result=`echo -e ${failcaseid} | tail -1|xargs`
    if [ "X${result}X" != "XX" ] && [ "X${result}X" != "XNOREPLACEX" ] && [ "X${result}X" != "XNORERUNX" ]; then
      echo -e "Found fail case ID: ${result}"
      SCENARIO="${result}"
    elif [ "X${result}X" == "XNORERUNX" ]; then
      echo "No need to rerun it"
      exit 0
    fi
  else
    echo "no launch name or invalid launch name, or not rerun pipeline build, and keep original ${SCENARIO}"
  fi

  echo -e "the scenario:\n${SCENARIO}"
}

function put_fake_launch_for_each_profile {
  if [ "${SCENARIO}" == "putfakelaunchforeachprofile" ] ; then
    ocpf ${WORKBUILDDIR} ${WORKSPACE} ${JENKINS_AGENT}
    exit 0
  fi
}

# remove openshift-tests repo, so hard-coded it here.
REPO_OWNER="openshift"
BRANCH="master"
run
