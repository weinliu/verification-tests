import { knmstateUtils } from "../../views/knmstate-utils";
import { nncpPage } from "../../views/nncp-page";
import { nnsPage } from "../../views/nns-page";

describe('knmstate operator console plugin related features', () => {

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.switchPerspective('Administrator');
    cy.log('delete nmstate instance and uninstall knmstate operator if existed before installing');
    knmstateUtils.deleteNMStateInstace();
    knmstateUtils.uninstall();
    knmstateUtils.install();
    knmstateUtils.createNMStateInstace();
  });

  it('(OCP-64784,qiowang) Verify NMState cosole plugin operator installation(GUI)', {tags: ['e2e','admin']}, () => {
    nncpPage.goToNNCP();
    cy.byLegacyTestID('resource-title').contains('NodeNetworkConfigurationPolicy');
    nnsPage.goToNNS();
    cy.byLegacyTestID('resource-title').contains('NodeNetworkState');
  });

  after(() => {
    knmstateUtils.deleteNMStateInstace();
    knmstateUtils.uninstall();
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.logout;
  });
  
});
