import { ClusterSettingPage } from './../../views/cluster-setting';
import { mcp } from "../../views/machine-config-pools";

describe("Improve MachineConfigPool list table for update status", () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env("LOGIN_USERNAME")}`);
    cy.login(Cypress.env("LOGIN_IDP"), Cypress.env("LOGIN_USERNAME"), Cypress.env("LOGIN_PASSWORD"));
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it("(OCP-51395,xiyuzhao,UserInterface) improve MachineConfigPool list table for update status", {tags: ['e2e','admin'] }, () => {
    const alertmsg = "Node updates are paused"
    ClusterSettingPage.goToClusterSettingDetails();
    ClusterSettingPage.editUpstreamConfig();
    ClusterSettingPage.configureChannel();

    mcp.listPage.goToMCPPage();
    cy.get('thead').should('contain','Update status');
    cy.get('[aria-label="MachineConfigPools"]').should('not.contain',/Updated|Updating|Paused/);
    mcp.listPage.checkAlertMsg('not.exist',alertmsg);
    cy.byTestID('name-filter-input')
      .clear()
      .type('worker')
      .then(() => {
        cy.get('button[data-test-id="kebab-button"]')
          .should('have.length', 1)
          .click();
        cy.byTestActionID('Pause updates').click();
      });
    mcp.listPage.checkUpdateStatus("worker", 'Paused');
    mcp.listPage.checkAlertMsg('contain',alertmsg);
    ClusterSettingPage.goToClusterSettingDetails();
    ClusterSettingPage.checkAlertMsg(alertmsg);

    cy.go('back');
    cy.get('[data-test-id="cluster-settings-alerts-paused-nodes"] button')
      .should('contain','Resume all updates')
      .click();
    mcp.listPage.checkUpdateStatus("worker", 'Up to date');
  });
});