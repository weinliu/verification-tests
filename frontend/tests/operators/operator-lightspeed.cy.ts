import { operatorHubPage } from "../../views/operator-hub-page"; 
describe('Lightspeed Operator related features', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });
  after(() => {
    cy.adminCLI(`oc delete CatalogSource lightspeedsc -n openshift-marketplace`, {failOnNonZeroExit: false});
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-76350,yanpzhan,UserInterface) Console could default select "Enable Operator recommended monitoring"',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    cy.exec(`oc get packagemanifests.packages.operators.coreos.com --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | grep lightspeed-operator`, {failOnNonZeroExit: false}).then((result) => {
      if(result.stdout.includes('lightspeed')){
	operatorHubPage.checkRecommenedMonitoring('lightspeed-operator', 'redhat-operators', 'OpenShift Lightspeed Operator', 'true');
      }else{
        cy.log("Lightspeed packagemanifest doesn't exit. Create it.");
        cy.adminCLI(`oc create -f ./fixtures/operators/lightspeed-cs.json`);
        cy.checkCommandResult(`oc get pod -l olm.catalogSource=lightspeedsc -n openshift-marketplace`, 'Running', { retries: 12, interval: 15000 });
	operatorHubPage.checkRecommenedMonitoring('lightspeed-operator', 'lightspeedsc', 'OpenShift Lightspeed Operator', 'true');
      }
    });
  });
})
