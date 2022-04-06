FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.17-openshift-4.11 AS builder
RUN mkdir -p /go/src/github.com/openshift/openshift-tests-private
WORKDIR /go/src/github.com/openshift/openshift-tests-private
COPY . .
RUN make build; \
    mkdir -p /tmp/build; \
    cp /go/src/github.com/openshift/openshift-tests-private/pipeline/handleresult.py /tmp/build/handleresult.py; \
    cp /go/src/github.com/openshift/openshift-tests-private/bin/extended-platform-tests /tmp/build/extended-platform-tests


FROM registry.ci.openshift.org/ocp/4.11:tools
COPY --from=builder /tmp/build/extended-platform-tests /usr/bin/
COPY --from=builder /tmp/build/handleresult.py /usr/bin/
RUN sh -c 'echo -e "[google-cloud-sdk]\nname=Google Cloud SDK\nbaseurl=https://packages.cloud.google.com/yum/repos/cloud-sdk-el7-x86_64\nenabled=1\ngpgcheck=1\nrepo_gpgcheck=1\ngpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg\n       https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg" > /etc/yum.repos.d/google-cloud-sdk.repo' && \
    PACKAGES="google-cloud-sdk git gzip zip util-linux openssh-clients httpd-tools make gcc" && \
    yum update -y && \
    yum install --setopt=tsflags=nodocs -y $PACKAGES && yum clean all && rm -rf /var/cache/yum/* && \
    git config --system user.name test-private && \
    git config --system user.email test-private@test.com && \
    chmod g+w /etc/passwd && \
    rm -rf /root/.config/gcloud
RUN pip3 install dotmap minio pyyaml requests
RUN curl -s -k https://dl.google.com/go/go1.17.6.linux-amd64.tar.gz -o go1.17.6.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.17.6.linux-amd64.tar.gz && rm -fr go1.17.6.linux-amd64.tar.gz && \
    mkdir -p /tmp/goproject && mkdir -p /tmp/gocache && chmod -R g=u /tmp/goproject && \
    chmod -R g+rw /tmp/goproject && chmod -R g=u /tmp/gocache && chmod -R g+rw /tmp/gocache && \
    oc image extract quay.io/openshifttest/hypershift-client:latest --file=/hypershift && mv hypershift /usr/bin/ && chmod 755 /usr/bin/hypershift && \
    oc image extract quay.io/openshifttest/openshift4-tools:v1 --file=/tmp/OpenShift4-tools.tar && tar -C /opt -xf OpenShift4-tools.tar && rm -fr OpenShift4-tools.tar && \
    curl -s -L https://github.com/vmware/govmomi/releases/latest/download/govc_Linux_x86_64.tar.gz -o govc_Linux_x86_64.tar.gz && tar -C /usr/bin/ -xvf govc_Linux_x86_64.tar.gz govc && rm -f govc_Linux_x86_64.tar.gz && \
    curl -s -k https://mirror2.openshift.com/pub/openshift-v4/clients/ocp-dev-preview/latest-4.11/opm-linux.tar.gz -o opm-linux.tar.gz && \
    tar -C /usr/bin -xzvf opm-linux.tar.gz && rm -fr opm-linux.tar.gz /usr/bin/oc && \
    curl -s -k https://mirror2.openshift.com/pub/openshift-v4/x86_64/clients/operator-sdk/ -o sdk.html && \
    curl -s -k https://mirror2.openshift.com/pub/openshift-v4/x86_64/clients/operator-sdk/pre-release/operator-sdk-linux-x86_64.tar.gz -o opt-sdk.tar.gz && \
    tar -C /usr/bin -xzvf opt-sdk.tar.gz && rm -fr opt-sdk.tar.gz
RUN curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" && \
    unzip awscliv2.zip && \
    ./aws/install -b /bin && \
    rm -rf ./aws awscliv2.zip
