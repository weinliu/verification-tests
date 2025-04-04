import { iscUtils } from "views/isc-utils";
import { metricsTab } from "views/metrics";
import { Pages } from "views/pages";
import { operatorHubPage } from "../../views/operator-hub-page";

describe('Operators related features', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.adminCLI(`oc delete fileintegrity --all -n openshift-file-integrity`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete project openshift-file-integrity`, { timeout: 60000, failOnNonZeroExit: false });
  });

  after(() => {
    cy.adminCLI(`oc delete fileintegrity example-fileintegrity -n openshift-file-integrity`, { failOnNonZeroExit: true });
    cy.adminCLI(`oc delete project openshift-file-integrity`, { timeout: 60000, failOnNonZeroExit: true });
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-59412,xiyuan,Security_and_Compliance) Install the file integrity operator through web console',{tags:['@smoke','@e2e','admin','@osd-ccs','@isc']}, () => {
    const params = {
      catalogsource: "",
      ns: "openshift-file-integrity",
      filename: "fileintegrity.yaml",
      operatorName: "File Integrity Operator"
    }

    // Set catalogsource name and install operator
    iscUtils.setCustomCatalogSource("file-integrity-operator").then((catalogsource) => {
      params.catalogsource = catalogsource;
      // Install the operator with the determined catalogsource
      operatorHubPage.installOperatorWithRecomendNamespace('file-integrity-operator', params.catalogsource);
      cy.get('[aria-valuetext="Loading..."]').should('exist');
      Pages.gotoInstalledOperatorPage('openshift-file-integrity')
      operatorHubPage.checkOperatorStatus(params.operatorName, 'Succeed');

      // check the file integrity oeprator pods
      cy.checkCommandResult(`oc get pod -l name=file-integrity-operator -n openshift-file-integrity`, 'Running', { retries: 12, interval: 5000 });

      //create a fileintegrity
      cy.exec(`oc apply -f ./fixtures/securityandcompliance/${params.filename} -n ${params.ns} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: true })
      .then(output => {
        expect(output.stderr).not.contain('Error');
      })
      cy.checkCommandResult(`oc get fileintegrity/example-fileintegrity -n openshift-file-integrity -o jsonpath='{.status.phase}'`, 'Active', {retries: 6, interval: 10000});

      //check the metrics
      cy.visit(`/monitoring/query-browser?query0=file_integrity_operator_daemonset_update_total`);
      cy.get('body').should('be.visible')
      metricsTab.checkMetricsLoadedWithoutError()
      cy.get('table[aria-label="query results table"]').should('exist');
    });
  });
})
