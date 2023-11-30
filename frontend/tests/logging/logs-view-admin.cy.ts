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
  };
  const LO = {
    namespace:   "openshift-operators-redhat",
    packageName: "loki-operator",
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
      logUtils.installOperator(CLO.namespace, CLO.packageName, csName, catalogSource.channel(CLO.packageName), "", true);
    });
    catalogSource.sourceName(LO.packageName).then((csName) => {
      logUtils.installOperator(LO.namespace, LO.packageName, csName, catalogSource.channel(LO.packageName));
    });

    //Create application logs
    cy.exec(`oc new-project ${Test_NS} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
    cy.exec(`oc new-app -f ./fixtures/logging/container_json_log_template.json -n ${Test_NS} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false});
    
    //Create bucket and secret for lokistack
    LokiUtils.prepareResourcesForLokiStack(CLO.namespace, LokiStack.secretName, LokiStack.bucketName);

    // Deploy lokistack under openshift-logging
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

    // Create LogFileMetricExporter
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/lfme.yaml -n ${CLO.namespace} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, {failOnNonZeroExit: false})
    .then(output => {
      expect(output.stderr).not.contain('Error');
    })

    logUtils.waitforPodReady(CLO.namespace, 'component=collector');
    logUtils.waitforPodReady(CLO.namespace, 'app.kubernetes.io/name=logging-view-plugin');
    logUtils.waitforPodReady(CLO.namespace, 'app.kubernetes.io/name=lokistack');
    logUtils.waitforPodReady(CLO.namespace, 'component=logfilesmetricexporter');

    // Verify that below metric is exposed when LFME is created
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
    cy.uiLogout();
  });

  it('(OCP-53324,gkarager) disable query executors when the query is empty', {tags: ['e2e','admin']}, () => {
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

  it('(OCP-53324,gkarager) LogQL Query', {tags: ['e2e','admin']}, () => {
    cy.visit(Logs_Page_URL);

    cy.byTestID(TestIds.LogsQueryInput).should('not.exist');
    cy.byTestID(TestIds.ShowQueryToggle).click();
    cy.byTestID(TestIds.LogsQueryInput).within(() => {
      cy.get('textarea').clear().type(`{ log_type=~".+", kubernetes_namespace_name="${Test_NS}" } | json`, {parseSpecialCharSequences: false})
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
    })
    const expectedData = [
      { term: 'log_type', description: 'application' },
      { term: 'kubernetes_namespace_name', description: `${Test_NS}` }
    ];   
    cy.get('.co-logs-detail_descripton-list').within(() => { 
      expectedData.forEach((expectedEntry) => { 
        cy.get(`.pf-c-description-list__term.co-logs-detail__list-term span.pf-c-description-list__text:contains('${expectedEntry.term}')`)
        .parents('.pf-c-description-list__group')
        .within(() => { 
          cy.get(`.pf-c-description-list__description .pf-c-description-list__text:contains('${expectedEntry.description}')`).should('exist');
        })      
      })
    })
  });
    
  it('(OCP-53324,gkarager) execute query as per selected tenant', {tags: ['e2e','admin']}, () => {
    const logType = "infrastructure"
    const expectedData = [
      { term: 'log_type', description: `${logType}` },
    ];   
    cy.visit(Logs_Page_URL);
    cy.byTestID(TestIds.TenantDropdown).click().within(() => {
      cy.contains(`${logType}`).click();
    });
    cy.get('table[aria-label="Logs Table"] tbody[role="rowgroup"]').within(() => {
      cy.get('tr.co-logs-table__row').should('not.be.empty');
    })
    cy.get('table[aria-label="Logs Table"]').within(() => {
      cy.get('tbody tr.co-logs-table__row').eq(0).find('button').click();
    })
    cy.get('.co-logs-detail_descripton-list').within(() => { 
      expectedData.forEach((expectedEntry) => { 
        cy.get(`.pf-c-description-list__term.co-logs-detail__list-term span.pf-c-description-list__text:contains('${expectedEntry.term}')`)
        .parents('.pf-c-description-list__group')
        .within(() => { 
          cy.get(`.pf-c-description-list__description .pf-c-description-list__text:contains('${expectedEntry.description}')`).should('exist');
        })      
      })
    })
  });
});
