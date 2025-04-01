import { guidedTour } from 'upstream/views/guided-tour';
import { listPage } from '../../upstream/views/list-page';
import { podsPage } from 'views/pods';
import { Pages } from 'views/pages';
import { userpage } from 'views/users';

const testns = "testproject-68476"
const [normalUser, normalUserPasswd] = Cypress.env('LOGIN_USERS').split(',')[0].split(':');
const [adminUser, adminUserPasswd] = Cypress.env('LOGIN_USERS').split(',')[1].split(':');

describe('pod log page', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${adminUser}`);
    cy.adminCLI(`oc new-project ${testns}`);
    cy.adminCLI(`oc create -f ./fixtures/pods/example-pod.yaml -n ${testns}`);
    cy.adminCLI(`oc create role test-pod-reader --namespace=${testns} --verb=get,list,watch --resource=pods,projects,namespaces`);
    cy.adminCLI(`oc create rolebinding test-pod-reader-binding --namespace=${testns} --user=${normalUser} --role=test-pod-reader`);
  })

  after(() => {
    cy.adminCLI(`oc delete project ${testns}`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${adminUser}`);
  })

  it('(OCP-68476,xiyuzhao,UserInterface)Excessive permissions in web-console impersonating a user',{tags:['@userinterface','@e2e','admin','@rosa','@osd-ccs']}, () => {
    const checkErrorAlertExistInLogsTab = () => {
      cy.contains('h4',/error|Danger|alert/gi)
        .should('exist')
        .parent()
        .find('button')
        .should('exist')
        .click();
      cy.get('h4').contains(/error|Danger|alert/gi);
    };
    // In Pod Details - Logs Tab for normal user:
    cy.uiLogin(Cypress.env('LOGIN_IDP'), normalUser, normalUserPasswd);
    guidedTour.close();
    podsPage.goToPodsLogTab(`${testns}`, "examplepod");
    checkErrorAlertExistInLogsTab();
    cy.uiLogout();
    // In Pod Details -> Logs Tab for the Cluster-admin who is impersonating the normal user:
    cy.uiLogin(Cypress.env('LOGIN_IDP'), adminUser, adminUserPasswd)
    Pages.gotoUsers();
    userpage.impersonateUser(normalUser);
    cy.clickNavLink(['Workloads', 'Deployments']);
    cy.byLegacyTestID('namespace-bar-dropdown').contains('Project:').click();
    cy.byTestID('dropdown-menu-item-link').contains(testns).click();
    cy.clickNavLink(['Workloads', 'Pods']);
    listPage.rows.shouldExist('examplepod').click();
    cy.get('[data-test-id="horizontal-link-Logs"]').should('exist').click();
    checkErrorAlertExistInLogsTab();
  })
})