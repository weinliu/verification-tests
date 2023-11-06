FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.20-openshift-4.15
RUN echo "it is for go to compile the binary to align with tests-private-base:4.15 from 4.15:tools"