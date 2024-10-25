import { metricsTab } from "views/metrics";
import { Pages } from "views/pages";
import { operatorHubPage } from "../../views/operator-hub-page";

describe('Operators related features', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.adminCLI(`oc delete ssb --all -n openshift-compliance`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete compliancesuite --all -n openshift-compliance`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete compliancescan --all -n openshift-compliance`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete pb --all -n openshift-compliance`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete project openshift-compliance`, { timeout: 60000, failOnNonZeroExit: false });
  });

  after(() => {
    cy.adminCLI(`oc delete ssb --all -n openshift-compliance`, { failOnNonZeroExit: true });
    cy.adminCLI(`oc delete compliancesuite --all -n openshift-compliance`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete compliancescan --all -n openshift-compliance`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc delete pb --all -n openshift-compliance`, {  timeout: 60000, failOnNonZeroExit: true });
    cy.adminCLI(`oc delete project openshift-compliance`, { timeout: 60000, failOnNonZeroExit: true });
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-59410,xiyuan,Security_and_Compliance) Install the Compliance Operator through web console',{tags:['@smoke','@e2e','admin','@osd-ccs']}, () => {
    const params = {
      ns: "openshift-compliance",
      filename: "ssb-cis.yaml",
      operatorName: "Compliance Operator"
    }

    // install compliance operator
    operatorHubPage.installOperatorWithRecomendNamespace('compliance-operator','qe-app-registry');
    cy.get('[aria-valuetext="Loading..."]').should('exist');
    Pages.gotoInstalledOperatorPage('openshift-compliance')
    operatorHubPage.checkOperatorStatus(params.operatorName, 'Succeed');

    // check the compliance oeprator pods
    cy.checkCommandResult(`oc get pod -l name=compliance-operator -n openshift-compliance`, 'Running', { retries: 6, interval: 10000 });
    cy.checkCommandResult(`oc get pb ocp4 -n openshift-compliance`, 'VALID', { retries: 30, interval: 10000 });
    cy.checkCommandResult(`oc get pb rhcos4 -n openshift-compliance`, 'VALID', { retries: 30, interval: 10000 });

    //create a ssb
    cy.exec(`oc apply -f ./fixtures/securityandcompliance/${params.filename} -n ${params.ns} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: true });
    cy.checkCommandResult(`oc get compliancesuite/ssb-cis-test -n openshift-compliance  -o=jsonpath='{.status.phase}'`, 'DONE', { retries: 30, interval: 10000 });

    //check the metrics compliance_operator_compliance_scan_status_total
    cy.visit(`/monitoring/query-browser?query0=compliance_operator_compliance_scan_status_total`);
    cy.get('body').should('be.visible')
    metricsTab.checkMetricsLoadedWithoutError()
    cy.get('table[aria-label="query results table"]').should('exist');
  });
})
