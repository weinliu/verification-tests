import { catalogSource, logUtils } from "../../views/logging-utils";
import { LokiUtils } from "views/logging-loki-utils";
import { TestIds } from "../../views/logs-view-page";
import { dashboard, graphSelector } from "views/dashboards-page"

describe('Logging view on the openshift console', () => {
  const Logs_Page_URL = '/monitoring/logs';
  const Test_NS = "test-ocp53324";
  const CLO = {
    namespace:   "openshift-logging",
    packageName: "cluster-logging",
    operatorName: "Red Hat OpenShift Logging"
  };
  const LO = {
    namespace:   "openshift-operators-redhat",
    packageName: "loki-operator",
    operatorName: "Loki Operator"
  };
  const LokiStack = {
    name:       "logging-lokistack-ocp53324",
    bucketName: "logging-loki-bucket-ocp53324",
    secretName: "logging-loki-secret-ocp53324",
    tSize:      "1x.demo"
  };  

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);    
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
 
    //Install logging operators if needed
    catalogSource.sourceName(CLO.packageName).then((csName) => {
      logUtils.installOperator(CLO.namespace, CLO.packageName, csName, catalogSource.channel(CLO.packageName), catalogSource.version(CLO.packageName), true, CLO.operatorName);
    });
    catalogSource.sourceName(LO.packageName).then((csName) => {
      logUtils.installOperator(LO.namespace, LO.packageName, csName, catalogSource.channel(LO.packageName), catalogSource.version(LO.packageName), false, LO.operatorName);
    });

    //Create application logs
    cy.exec(`oc new-project ${Test_NS} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
    cy.exec(`oc new-app -f ./fixtures/logging/container_json_log_template.json -n ${Test_NS} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
    
    //Create bucket and secret for lokistack
    LokiUtils.prepareResourcesForLokiStack(CLO.namespace, LokiStack.secretName, LokiStack.bucketName);

    //Deploy lokistack under openshift-logging
    LokiUtils.getStorageClass().then((SC) => { 
      LokiUtils.getPlatform()
      cy.get<string>('@ST').then(ST => { 
        cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/lokistack-sample.yaml -n ${CLO.namespace} -p NAME=${LokiStack.name} SIZE=${LokiStack.tSize} SECRET_NAME=${LokiStack.secretName} STORAGE_TYPE=${ST} STORAGE_CLASS=${SC} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, {failOnNonZeroExit: false})
        .then(output => { 
          expect(output.stderr).not.contain('Error');
        })
      })
    })
    
    //Create CL instance
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/cl_logStore_lokistack.yaml -n ${CLO.namespace} -p LOKISTACKNAME=${LokiStack.name} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, {failOnNonZeroExit: false})
    .then(output => { 
      expect(output.stderr).not.contain('Error');
    })

    //Create LogFileMetricExporter
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/lfme.yaml -n ${CLO.namespace} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })

    logUtils.waitforPodReady(CLO.namespace, 'component=collector');
    logUtils.waitforPodReady(CLO.namespace, 'app.kubernetes.io/name=logging-view-plugin');
    logUtils.waitforPodReady(CLO.namespace, 'app.kubernetes.io/name=lokistack');
    logUtils.waitforPodReady(CLO.namespace, 'component=logfilesmetricexporter');

    //Verify that below metric is exposed when LFME is created
    dashboard.visitDashboard('grafana-dashboard-cluster-logging');
    cy.byLegacyTestID('panel-produced-logs').should('exist').within(() => {
      cy.byTestID('top-producing-containers-chart').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')
    });
  });

  after(() => {
    cy.exec(`oc delete project ${Test_NS} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
    cy.exec(`oc delete secret ${LokiStack.secretName} -n ${LO.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
    LokiUtils.removeObjectStorage(LokiStack.bucketName);
    logUtils.removeLokistack(LokiStack.name, CLO.namespace)
    logUtils.removeClusterLogging(CLO.namespace)
    cy.exec(`oc delete lfme/instance -n ${CLO.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-53324,gkarager,Logging) disable query executors when the query is empty', {tags: ['e2e','admin','@logging']}, () => {
    cy.visit(Logs_Page_URL);
    cy.byTestID(TestIds.RefreshIntervalDropdown).should('exist');
    cy.byTestID(TestIds.TimeRangeDropdown).should('exist');
    cy.byTestID(TestIds.SyncButton).should('exist');
    cy.byTestID(TestIds.LogsTable).should('exist')
    cy.byTestID(TestIds.ToggleHistogramButton).click();
    cy.byTestID(TestIds.LogsHistogram).should('exist')

    cy.byTestID(TestIds.ShowQueryToggle).click();
    cy.byTestID(TestIds.LogsQueryInput).within(() => {
      cy.get('textarea').clear();
    });
    cy.byTestID(TestIds.ExecuteQueryButton).should('be.disabled');
    cy.byTestID(TestIds.RefreshIntervalDropdown).within(() => {
      cy.get('button').should('be.disabled');
    });
    cy.byTestID(TestIds.TimeRangeDropdown).within(() => {
      cy.get('button').should('be.disabled');
    });
    cy.byTestID(TestIds.SyncButton).should('be.disabled');
    cy.byTestID(TestIds.SeverityDropdown).within(() => {
      cy.get('button').should('be.disabled');
    });
    cy.byTestID(TestIds.TenantDropdown).within(() => {
      cy.get('button').should('be.disabled');
    });
  }); 

  it('(OCP-53324,gkarager,Logging) LogQL Query', {tags: ['e2e','admin','@logging', '@smoke','@level0']}, () => {
    cy.visit(Logs_Page_URL);

    cy.byTestID(TestIds.LogsQueryInput).should('not.exist');
    cy.byTestID(TestIds.ShowQueryToggle).click();
    cy.byTestID(TestIds.LogsQueryInput).within(() => {
      cy.get('textarea').clear().type(`{ log_type="application", kubernetes_namespace_name="${Test_NS}" } | json`, {parseSpecialCharSequences: false})
    });
    cy.byTestID(TestIds.ExecuteQueryButton).click();
    cy.byTestID(TestIds.RefreshIntervalDropdown).click().within(() => {
      cy.get('[id="15s"]').click();
    })
    cy.get('table[aria-label="Logs Table"] tbody[role="rowgroup"]').within(() => {
      cy.get('tr.co-logs-table__row').should('not.be.empty');
    })
    cy.get('table[aria-label="Logs Table"]').within(() => {
      cy.get('tbody tr.co-logs-table__row').eq(0).find('button').click();
      cy.get('.co-logs-detail_descripton-list').within(() => {
        cy.get('.pf-c-description-list__text').should('contain.text', `${Test_NS}`);
        cy.get('.pf-c-description-list__text').should('contain.text', 'application');
      })
    })
  });

  it('(OCP-53324,gkarager,Logging) execute query as per selected tenant', {tags: ['e2e','admin','@logging']}, () => {
    const logType = "infrastructure"
    cy.visit(Logs_Page_URL);
    cy.byTestID(TestIds.TenantDropdown).click().within(() => {
      cy.contains(`${logType}`).click();
    });
    cy.get('table[aria-label="Logs Table"] tbody[role="rowgroup"]').within(() => {
      cy.get('tr.co-logs-table__row').should('not.be.empty');
    })
    cy.get('table[aria-label="Logs Table"]').within(() => {
      cy.get('tbody tr.co-logs-table__row').eq(0).find('button').click();
      cy.get('.co-logs-detail_descripton-list').within(() => {
        cy.get('.pf-c-description-list__text').should('contain.text', `${logType}`);
      })
    })
  })
});
