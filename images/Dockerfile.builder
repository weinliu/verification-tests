FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.22-openshift-4.18
RUN echo "it is for go to compile the binary to align with tests-private-base:4.18 from 4.18:tools"
