import { ClusterSettingPage } from './../../views/cluster-setting';
import { mcp } from "../../views/machine-config-pools";
import { Pages } from 'views/pages';

describe("Improve MachineConfigPool list table for update status", () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env("LOGIN_USERNAME")}`);
    cy.uiLogin(Cypress.env("LOGIN_IDP"), Cypress.env("LOGIN_USERNAME"), Cypress.env("LOGIN_PASSWORD"));
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it("(OCP-51395,xiyuzhao,UserInterface) improve MachineConfigPool list table for update status",{tags:['@userinterface','@e2e','admin','@destructive']}, () => {
    ClusterSettingPage.goToClusterSettingDetails();
    ClusterSettingPage.editUpstreamConfig();
    ClusterSettingPage.configureChannel();
    //Check column 'update status', and actions in Kebab list in MCP list page
    Pages.gotoMCPListPage();
    cy.get('thead').should('contain','Update status');
    cy.get('[aria-label="MachineConfigPools"]').should('not.contain',/Updated|Updating|Paused/);
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
    //Check Alert in MCP Details Page
    Pages.gotoMCPDetailsPage('worker')
    cy.contains('[class*="-alert__action-group"] button', 'Resume updates')
      .click()
      .then(() => {
        cy.get('[class*="alert__title"]').should('not.exist');
        Pages.gotoMCPListPage();
        mcp.listPage.checkUpdateStatus("worker", /Updating|Up to date/);
      })
  });
});