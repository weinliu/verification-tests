import { operatorHubPage } from "../../views/operator-hub-page";
import { Pages } from "../../views/pages";

describe('Lightspeed related features', () => {
  const OLS = {
    namespace:   "openshift-lightspeed",
    packageName: "lightspeed-operator",
    operatorName: "OpenShift Lightspeed Operator"
  };

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    //Delete operator if already exists
    Pages.gotoInstalledOperatorPage(OLS.namespace);
    cy.exec(`oc get sub ${OLS.packageName} -n ${OLS.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
      if(!result.stderr.includes('NotFound')){
      operatorHubPage.removeOperator(OLS.operatorName);
      }
    });
  });

  after(() => {
    Pages.gotoInstalledOperatorPage(OLS.namespace);
    operatorHubPage.checkOperatorStatus(OLS.operatorName, "Succeeded")
    operatorHubPage.removeOperator(OLS.operatorName);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OLS-427,jfula,Lightspeed) Deploy Openshift Lightspeed operator via web console', {tags: ['e2e','admin', '@smoke']}, () => {
    operatorHubPage.installOperator(OLS.packageName, "redhat-operators");

    //Install the Openshift Lightspeed catalog source
    // lightUtils.installOperator(OLS.namespace, OLS.packageName, "redhat-operators", catalogSource.channel(OLS.packageName), catalogSource.version(OLS.packageName), false, OLS.operatorName);
    //Install the Openshift Lightspeed Operator with console plungin
  });
});
