FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.20-openshift-4.15 AS builder
RUN mkdir -p /go/src/github.com/openshift/openshift-tests-private
WORKDIR /go/src/github.com/openshift/openshift-tests-private
COPY . .
RUN make go-mod-tidy; \
    make build; \
    mkdir -p /tmp/build; \
    cp /go/src/github.com/openshift/openshift-tests-private/pipeline/handleresult.py /tmp/build/handleresult.py; \
    cp /go/src/github.com/openshift/openshift-tests-private/bin/extended-platform-tests /tmp/build/extended-platform-tests


FROM registry.ci.openshift.org/ocp/4.15:tools
COPY --from=builder /tmp/build/extended-platform-tests /usr/bin/
COPY --from=builder /tmp/build/handleresult.py /usr/bin/
RUN sh -c 'echo -e "[google-cloud-sdk]\nname=Google Cloud SDK\nbaseurl=https://packages.cloud.google.com/yum/repos/cloud-sdk-el7-x86_64\nenabled=1\ngpgcheck=1\nrepo_gpgcheck=1\ngpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg\n       https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg" > /etc/yum.repos.d/google-cloud-sdk.repo' && \
    sh -c 'echo -e "[azure-cli]\nname=Azure CLI\nbaseurl=https://packages.microsoft.com/yumrepos/azure-cli\nenabled=1\ngpgcheck=1\ngpgkey=https://packages.microsoft.com/keys/microsoft.asc" >/etc/yum.repos.d/azure-cli.repo' && \
    PACKAGES="azure-cli google-cloud-sdk git gzip zip util-linux openssh-clients httpd-tools skopeo java-11-openjdk-headless" && \
    yum update -y && \
    yum install --setopt=tsflags=nodocs -y $PACKAGES && yum clean all && rm -rf /var/cache/yum/* && \
    git config --system user.name test-private && \
    git config --system user.email test-private@test.com && \
    chmod g+w /etc/passwd && \
    rm -rf /root/.config/gcloud
RUN pip3 install --upgrade setuptools pip && pip3 install dotmap minio pyyaml requests python-openstackclient && \
    sh -c 'echo -e "[rhel8.7-baseos]\nname=rhel8.7-baseos\nbaseurl=http://download.eng.rdu.redhat.com/rhel-8/rel-eng/RHEL-8/latest-RHEL-8.7/compose/BaseOS/x86_64/os\nenabled=0\ngpgcheck=1" >/etc/yum.repos.d/rhel8.7-baseos.repo' && \
    yum -y --enablerepo=rhel8.7-baseos install sos
RUN MIRRORURL=https://mirror2.openshift.com/pub/openshift-v4 && CLIENTURL=${MIRRORURL}/x86_64/clients/ocp/candidate && \
    curl -s -k -L ${MIRRORURL}/x86_64/clients/ocp/ -o ocp.html && curl -s -k -L ${MIRRORURL}/x86_64/clients/ocp-dev-preview/ -o pre.html && \
    ecver=$(grep -E "<a href=\"candidate-4\.15" pre.html |cut -d"\"" -f2|cut -d"/" -f1|sort -V|tail -1) && echo "V${ecver}V" && \
    if [ "V${ecver}V" != "VV"  ]; then CLIENTURL=${MIRRORURL}/x86_64/clients/ocp-dev-preview/${ecver}; fi && \
    rcgaver=$(grep -E "<a href=\"4\.15" ocp.html |cut -d"\"" -f2|cut -d"/" -f1|sort -V|tail -1) && echo "V${rcgaver}V" && \
    if [ "V${rcgaver}V" != "VV"  ]; then CLIENTURL=${MIRRORURL}/x86_64/clients/ocp/${rcgaver}; fi && \
    curl -s -k -L ${CLIENTURL}/openshift-client-linux.tar.gz -o openshift-client-linux.tar.gz && \
    curl -s -k -L ${CLIENTURL}/opm-linux.tar.gz -o opm-linux.tar.gz  && \
    curl -s -k -L ${CLIENTURL}/oc-mirror.tar.gz -o oc-mirror.tar.gz && \
    tar -C /usr/bin -xzvf openshift-client-linux.tar.gz && rm -fr openshift-client-linux.tar.gz && \
    tar -C /usr/bin -xzvf opm-linux.tar.gz && rm -fr opm-linux.tar.gz && \
    tar -C /usr/bin/ -xzvf oc-mirror.tar.gz && chmod +x /usr/bin/oc-mirror && rm -f oc-mirror.tar.gz && \
    SDKURL=${MIRRORURL}/x86_64/clients/operator-sdk/pre-release && \
    curl -s -k -L ${MIRRORURL}/x86_64/clients/operator-sdk/ -o sdk.html && \
    optsdkver=$(grep -E "<a href=\"4\.15" sdk.html |cut -d"\"" -f2|cut -d"/" -f1|sort -V|tail -1) && echo ${optsdkver} && \
    if [ "V${optsdkver}V" != "VV"  ]; then SDKURL=${MIRRORURL}/x86_64/clients/operator-sdk/${optsdkver}; fi && \
    curl -s -k -L ${SDKURL}/operator-sdk-linux-x86_64.tar.gz -o opt-sdk.tar.gz && \
    tar -C ./ -xzvf opt-sdk.tar.gz && mv ./x86_64/operator-sdk /usr/bin && rm -fr opt-sdk.tar.gz ./x86_64 && \
    echo "try to install binary used by the cases" && oc version --client && \
    oc image extract quay.io/openshifttest/hypershift-client:latest --file=/hypershift && mv hypershift /usr/bin/ && chmod 755 /usr/bin/hypershift && \
    curl -s -L https://mirror.openshift.com/pub/openshift-v4/clients/rosa/latest/rosa-linux.tar.gz -o rosa.tar.gz && tar -C /usr/bin -xf rosa.tar.gz && chmod 755 /usr/bin/rosa && rosa version && rm -fr rosa.tar.gz && \
    oc image extract quay.io/openshifttest/oc-compliance:latest --file /tmp/oc-compliance && mv oc-compliance /usr/bin/ && chmod 755 /usr/bin/oc-compliance && \
    oc image extract quay.io/openshifttest/openshift4-tools:v1 --file=/tmp/OpenShift4-tools.tar && tar -C /opt -xf OpenShift4-tools.tar && rm -fr OpenShift4-tools.tar && \
    curl -s -L https://github.com/vmware/govmomi/releases/latest/download/govc_Linux_x86_64.tar.gz -o govc_Linux_x86_64.tar.gz && tar -C /usr/bin/ -xvf govc_Linux_x86_64.tar.gz govc && rm -f govc_Linux_x86_64.tar.gz
RUN curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" && \
    unzip -q awscliv2.zip && \
    ./aws/install -b /bin && \
    rm -rf ./aws awscliv2.zip /usr/bin/oc /usr/bin/kubectl
