FROM fedora:latest

# Install Golang
RUN dnf -y update && \
    dnf -y install golang wget

ARG YQ_VERSION="v4.30.8"

# Install kubectl and oc
RUN curl -L -o oc.tar.gz https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/latest/openshift-client-linux.tar.gz \
    && tar -xvzf oc.tar.gz \
    && chmod +x kubectl oc \
    && mv oc kubectl /usr/local/bin/

RUN set -x && \
    curl --silent --location https://rpm.nodesource.com/setup_lts.x | bash - && \
    curl --silent --location https://dl.yarnpkg.com/rpm/yarn.repo | tee /etc/yum.repos.d/yarn.repo && \
    PACKAGES="openssh-clients httpd-tools nodejs yarn xorg-x11-server-Xvfb gtk2-devel gtk3-devel libnotify-devel nss libXScrnSaver alsa-lib" && \
    yum install --setopt=tsflags=nodocs -y $PACKAGES && \
    declare -A YQ_HASH=([amd64]='6c911103e0dcc54e2ba07e767d2d62bcfc77452b39ebaee45b1c46f062f4fd26' \
                        [arm64]='95092e8b5332890c46689679b5e4360d96873c025ad8bafd961688f28ea434c7') && \
    arch="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" && \
    YQ_URI="https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${arch}" && \
    curl -sSL "${YQ_URI}" -o /usr/local/bin/yq && \
    echo "${YQ_HASH[$arch]} */usr/local/bin/yq" | sha256sum --strict --status --check && \
    chmod +x /usr/local/bin/yq && \
    yum clean all && rm -rf /var/cache/yum/*

RUN wget https://dl.google.com/linux/direct/google-chrome-stable_current_x86_64.rpm && \
    yum install -y ./google-chrome-stable_current_*.rpm && \
    rm ./google-chrome-stable_current_*.rpm && \
    mkdir -p /go/src/github.com/openshift

# Copy current context to the specified directory
COPY . /go/src/github.com/openshift/openshift-tests-private

WORKDIR /go/src/github.com/openshift/openshift-tests-private