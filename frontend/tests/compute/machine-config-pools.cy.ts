import { listPage } from './../../upstream/views/list-page';
import { ClusterSettingPage } from './../../views/cluster-setting';
import { mcp } from "../../views/machine-config-pools";

describe("Improve MachineConfigPool list table for update status", () => {
  const params = {
    fileName: 'machine-config-pools',
    testName: 'infra-test',
    testMCPName: 'infra-test'
  };
  const {fileName, testMCPName} = params;

  before(() => {
    cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env("LOGIN_USERNAME")} --kubeconfig ${Cypress.env("KUBECONFIG_PATH")}`);
    cy.exec(`oc create -f ./fixtures/${fileName}.yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.login(Cypress.env("LOGIN_IDP"), Cypress.env("LOGIN_USERNAME"), Cypress.env("LOGIN_PASSWORD"));
  });
  
  after(() => {
    cy.exec(`oc delete machineconfigpools ${params.testName} --kubeconfig ${Cypress.env("KUBECONFIG_PATH")}`);
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.logout();
  });

  it("(OCP-51395, xiangyli) Improve MachineConfigPool list table for update status", {tags: ['e2e','admin'] }, () => {

    cy.visit('/settings/cluster')
    ClusterSettingPage.configureChannel()
    ClusterSettingPage.editUpstreamConfig()
    cy.visit('/settings/cluster')
    cy.contains(/Up to date|Available updates/g).should('be.visible')

    mcp.listPage.goToMCPPage()
    // Old columns should not exist
    cy.get('thead').contains('Update status').should('be.visible')
      .contains(/Updated|Updating|Paused/).should('not.exist')

    cy.get("Node updates are paused").should("not.exist")
    
    listPage.rows.clickKebabAction(testMCPName, 'Pause updates')
    cy.contains('Node updates are paused').should('be.visible')
    mcp.listPage.checkUpdateStatus(testMCPName, 'Paused')
    
    cy.visit('/settings/cluster')
    cy.contains('Node updates are paused').should('be.visible')
    
    mcp.listPage.goToMCPPage()
    cy.byLegacyTestID('cluster-settings-alerts-paused-nodes').within(() => {
      cy.byButtonText('Resume all updates').click()
    })
    mcp.listPage.checkUpdateStatus(testMCPName, 'Up to date')
  });
});
