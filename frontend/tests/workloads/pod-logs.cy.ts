import { guidedTour } from 'upstream/views/guided-tour';
import { listPage } from '../../upstream/views/list-page';
import { podsPage } from 'views/pods';
import { Pages } from 'views/pages';
import { userpage } from 'views/users';
import { projectDropdown } from 'upstream/views/common';
const testns = "testproject-68476"
let normal_user:any, normal_user_passwd:any, admin_user:any, admin_user_passwd:any;

describe('pod log page', () => {
  before(() => {
    const up_pair = Cypress.env('LOGIN_USERS').split(',');
    const [a, b] = up_pair;
    normal_user = a.split(':')[0];
    normal_user_passwd = a.split(':')[1];
    admin_user = b.split(':')[0];
    admin_user_passwd = b.split(':')[1];

    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${admin_user}`);
    cy.exec(`oc login -u ${admin_user} -p ${admin_user_passwd} ${Cypress.env('HOST_API')} --insecure-skip-tls-verify=true`, { failOnNonZeroExit: false })
    cy.adminCLI(`oc new-project ${testns}`);
    cy.adminCLI(`oc create -f ./fixtures/pods/example-pod.yaml -n ${testns}`);
    cy.adminCLI(`oc create role test-pod-reader --namespace=${testns} --verb=get,list,watch --resource=pods,projects,namespaces`);
    cy.adminCLI(`oc create rolebinding test-pod-reader-binding --namespace=${testns} --user=${normal_user} --role=test-pod-reader`);
  })

  after(() => {
    cy.adminCLI(`oc delete project ${testns}`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${admin_user}`);
  })

  it('(OCP-68476,xiyuzhao,UserInterface)Excessive permissions in web-console impersonating a user', {tags:['e2e','admin','@rosa','@osd-ccs']}, () => {
    const checkErrorAlertExistInLogsTab = () => {
      cy.contains('h4',/error|Danger|alert/gi)
        .should('exist')
        .parent()
        .find('button')
        .should('exist')
        .click();
      cy.get('h4').contains(/error|Danger|alert/gi);
    };

    /* When normal user do not have the privilege to the pod/logs resource
       Then Pods log cannot be loaded
       In Command Line: */
    cy.exec(`oc login -u ${normal_user} -p ${normal_user_passwd} ${Cypress.env('HOST_API')} --insecure-skip-tls-verify=true`, { failOnNonZeroExit: false })
    cy.exec(`oc logs -f examplepod`, {failOnNonZeroExit: false})
      .then(output => {
         expect(output.stderr).contain('Forbidden');
    });

    // In Pod Details - Logs Tab for normal user:
    cy.login(Cypress.env('LOGIN_IDP'), normal_user, normal_user_passwd);
    guidedTour.close();
    podsPage.goToPodsLogTab(`${testns}`, "examplepod");
    checkErrorAlertExistInLogsTab();

    // In Pod Details -> Logs Tab for the Cluster-admin who is impersonating the normal user:
    cy.login(Cypress.env('LOGIN_IDP'), admin_user, admin_user_passwd)
    Pages.gotoUsers();
    userpage.impersonateUser(normal_user);
    cy.switchPerspective('Administrator');
    cy.clickNavLink(['Workloads', 'Pods']);
    cy.byLegacyTestID('namespace-bar-dropdown').contains('Project:').click();
    cy.byTestID('dropdown-menu-item-link').contains(testns).click();
    listPage.rows.shouldExist('examplepod').click();
    cy.get('[data-test-id="horizontal-link-Logs"]').should('exist').click();
    checkErrorAlertExistInLogsTab();
  })
})