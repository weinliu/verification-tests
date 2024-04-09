FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.21-openshift-4.16
RUN echo "it is for go to compile the binary to align with tests-private-base:4.16 from 4.16:tools"
