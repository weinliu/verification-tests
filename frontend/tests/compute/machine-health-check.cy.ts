import { nodesPage } from "views/nodes";
import { Pages } from "views/pages";
describe('nodes page', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });
  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });
  it('(OCP-70839,yanpzhan,UserInterface) Node overview page displays well when related machinehealthcheck does not set spec.unhealthyConditions', {tags: ['e2e','admin']}, function () {
    cy.isIPICluster().then(value => {
      if(value == false){
        this.skip();
      }
    });
    cy.adminCLI(`oc create -f ./fixtures/mhc-70833.yaml`);
    cy.adminCLI(`oc get node -l node-role.kubernetes.io/worker -ojsonpath={'.items[0].metadata.name'}`).then(result => {
      let nodeName = result.stdout;
      Pages.gotoNodeOverviewPage(nodeName);
    });
    nodesPage.checkMachineHealthCheck('test-mhc');
    cy.adminCLI('oc delete machinehealthchecks.machine.openshift.io test-mhc -n openshift-machine-api');
  });
})

