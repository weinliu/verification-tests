import { Pages } from 'views/pages';
import { listPage } from "../../upstream/views/list-page";
import { guidedTour } from 'upstream/views/guided-tour';
import { detailsPage } from 'upstream/views/details-page';
import { nav } from 'upstream/views/nav';

const groupName = 'manager';
const clusterrole_name = 'auto-metrics-reader';
const clusterrb_name = 'auto-metrics-rb';

describe('Group tests', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI('oc create -f ./fixtures/rbac/group.yaml');
    cy.adminCLI('oc create -f ./fixtures/rbac/metrics-reader-cluster-role.yaml');
    cy.adminCLI('oc create -f ./fixtures/rbac/metrics-reader-cluster-rb.yaml');
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc delete clusterrolebinding ${clusterrb_name}`);
    cy.adminCLI(`oc delete clusterrole ${clusterrole_name}`);
    cy.adminCLI(`oc delete group ${groupName}`);
  });

  it('(OCP-72434,yapei,UserInterface)Add Impersonate Group action to Groups list and details', {tags: ['e2e','admin']}, () => {
    // verify Impersonate Group action on list page
    Pages.gotoGroupListPage();
    listPage.rows.clickKebabAction(groupName,`Impersonate Group ${groupName}`);
    cy.get('[data-test="global-notifications"]', {timeout: 20000}).contains('You are impersonating').as('impersonate_message').should('exist');
    guidedTour.close();
    cy.switchPerspective('Administrator');
    nav.sidenav.shouldHaveNavSection(['Observe']);
    nav.sidenav.clickNavLink(['User Management']);
    cy.get('a[data-test="nav"]').contains('Users').should('not.exist');
    cy.get('a[data-test="nav"]').contains('Groups').should('not.exist');

    // verify impersonating has been correctly stoped
    cy.byButtonText('Stop impersonation').click();
    cy.get('@impersonate_message').should('not.exist');
    nav.sidenav.clickNavLink(['User Management']);
    cy.get('a[data-test="nav"]').contains('Users').should('exist');
    cy.get('a[data-test="nav"]').contains('Groups').should('exist');
    // verify Impersonate Group action on details page
    Pages.gotoOneGroupPage(groupName);
    detailsPage.clickPageActionFromDropdown(`Impersonate Group ${groupName}`);
    cy.get('@impersonate_message').should('exist');
    cy.byButtonText('Stop impersonation').should('exist').click();
  });
})