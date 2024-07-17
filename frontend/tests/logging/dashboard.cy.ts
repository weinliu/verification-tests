import { catalogSource, logUtils } from "../../views/logging-utils";
import { dashboard, graphSelector } from "views/dashboards-page"
describe('Logging related features', () => {
  const CLO = {
    namespace:   "openshift-logging",
    packageName: "cluster-logging",
    operatorName: "Red Hat OpenShift Logging"
  };
  const EO = {
    namespace:   "openshift-operators-redhat",
    packageName: "elasticsearch-operator",
    operatorName: "OpenShift Elasticsearch Operator"
  };
  const LO = {
    namespace:   "openshift-operators-redhat",
    packageName: "loki-operator",
    operatorName: "Loki Operator"
  };
  const Test_NS = "cluster-logging-dashboard-test";

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    // Install logging operators if needed
    catalogSource.sourceName(CLO.packageName).then((csName) => {
      logUtils.installOperator(CLO.namespace, CLO.packageName, csName, catalogSource.channel(CLO.packageName), catalogSource.version(CLO.packageName), true, CLO.operatorName);
    });
    catalogSource.sourceName(EO.packageName).then((csName) => {
      logUtils.installOperator(EO.namespace, EO.packageName, csName, catalogSource.channel(EO.packageName), catalogSource.version(EO.packageName), false, EO.operatorName);
    });
    catalogSource.sourceName(LO.packageName).then((csName) => {
      logUtils.installOperator(LO.namespace, LO.packageName, csName, catalogSource.channel(LO.packageName), catalogSource.version(LO.packageName), false, LO.operatorName);
    });

    cy.exec(`oc new-project ${Test_NS} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
    cy.exec(`oc label ns ${Test_NS} openshift.io/cluster-monitoring=true --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
    cy.exec(`oc -n ${Test_NS} create role prometheus-k8s --verb=get,list,watch --resource=pods,services,endpoints --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
    cy.exec(`oc -n ${Test_NS} policy add-role-to-user --role-namespace=${Test_NS} prometheus-k8s system:serviceaccount:openshift-monitoring:prometheus-k8s --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
  });

  after(() => {
    cy.exec(`oc delete project ${Test_NS} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
    cy.exec(`oc delete lfme/instance -n ${CLO.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
    logUtils.removeClusterLogging(CLO.namespace);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.logout;
  });

  it('(OCP-66825,qitang,Logging) Vector - OpenShift Logging Collection Vector metrics dashboard', {tags: ['e2e','admin','@logging']}, () => {
    // Create Logging instance
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/cl_default_es.yaml -n ${CLO.namespace} -p COLLECTOR=fluentd | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })
    // Create LogFileMetricExporter
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/lfme.yaml -n ${CLO.namespace} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })

    // Deploy self-managed loki in a new project
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/loki-configmap.yaml -n ${Test_NS} -p NAME=loki-server -p NAMESPACE=${Test_NS} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/loki-deployment.yaml -n ${Test_NS} -p NAME=loki-server -p NAMESPACE=${Test_NS} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -n ${Test_NS} expose deployment/loki-server`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })

    // Create multi CLF
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -n ${Test_NS} create sa test-66825`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -n ${Test_NS} adm policy add-cluster-role-to-user collect-application-logs -z test-66825`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -n ${Test_NS} adm policy add-cluster-role-to-user collect-infrastructure-logs -z test-66825`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -n ${Test_NS} adm policy add-cluster-role-to-user collect-audit-logs -z test-66825`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/clf-external-loki.yaml -n ${Test_NS} -p NAME=collector-loki-server -p NAMESPACE=${Test_NS} -p URL=http://loki-server.${Test_NS}.svc:3100 -p SERVICE_ACCOUNT_NAME=test-66825 | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })

    // Wait for logging pods to be ready
    logUtils.waitforPodReady(CLO.namespace, 'component=elasticsearch');
    logUtils.waitforPodReady(CLO.namespace, 'component=kibana');
    logUtils.waitforPodReady(CLO.namespace, 'component=collector');
    logUtils.waitforPodReady(CLO.namespace, 'component=logfilesmetricexporter');
    logUtils.waitforPodReady(Test_NS, 'component=collector');

    // Check Logging/Collection dashboard
    dashboard.visitDashboard('grafana-dashboard-cluster-logging');

    cy.byLegacyTestID('panel-overview').should('exist').within(() => {
      cy.byTestID('log-bytes-collected-(24h-avg)-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state');
      cy.byTestID('log-bytes-sent-(24h-avg)-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('log-collection-rate-(5m-avg)-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('log-send-rate-(5m-avg)-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('total-errors-last-60m-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    });

    cy.byLegacyTestID('panel-outputs').should('exist').within(() => {
        cy.byTestID('rate-log-bytes-sent-per-output-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    });

    cy.byLegacyTestID('panel-produced-logs').should('exist').within(() => {
        cy.byTestID('top-producing-containers-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
        cy.byTestID('top-producing-containers-in-last-24-hours-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    });

    cy.byLegacyTestID('panel-collected-logs').should('exist').within(() => {
      cy.byTestID('top-collected-containers---bytes/second-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('top-collected-containers-in-last-24-hours-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
  });

    cy.byLegacyTestID('panel-machine').should('exist').within(() => {
      cy.byTestID('cpu-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('memory-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('running-containers-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('open-files-for-container-logs-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
      cy.byTestID('file-descriptors-in-use-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    });
  });

});
