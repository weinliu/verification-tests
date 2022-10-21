import { Overview } from '../../views/overview';
import { preferNotifications } from '../../views/user-preferences';
import testConfigMap from '../../fixtures/cluster-monitoring-config.json';
import testAlert from '../../fixtures/testalert.json';
describe('Notification drawer tests', () => {
  let $cmexisting = 0;
  before(() => {
    cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc new-project test-ocp45305 --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc get cm cluster-monitoring-config -n openshift-monitoring --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result) => {
      const $ret = result.code;
      if($ret == 0){
        $cmexisting = 1;
        cy.exec(`oc patch cm cluster-monitoring-config -n openshift-monitoring --type='merge' -p '{"data": {"config.yaml": "enableUserWorkload: true"}}' --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false });
      }
      if($ret == 1){
        cy.exec(`echo '${JSON.stringify(testConfigMap)}' | oc create -f - --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
      }
    })
//    cy.exec(`oc label namespace test-ocp45305 openshift.io/cluster-monitoring=true --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`echo '${JSON.stringify(testAlert)}' | oc create -f - --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -n test-ocp45305`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  })

  after(() => {
    cy.exec(`oc delete project test-ocp45305 --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    if ($cmexisting == 1){
      cy.exec(`oc patch cm cluster-monitoring-config -n openshift-monitoring --type='json' -p='[{"op": "remove", "path": "/data/config.yaml"}]' --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    }else{
      cy.exec(`oc delete cm cluster-monitoring-config -n openshift-monitoring --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    }
    cy.logout;
  })

  it('(OCP-45305,yanpzhan) check alert on overview page and notification drawer list', {tags: ['e2e','admin']}, () => {
    preferNotifications.goToNotificationsTab();
    preferNotifications.setHideNotifications();
    Overview.goToDashboard();
    Overview.isLoaded();
    cy.contains('Testing 123').should('not.exist')
    Overview.clickNotificationDrawer();
    cy.contains('Testing 123').should('not.exist');
    preferNotifications.goToNotificationsTab();
    cy.contains('Hide user workload notifications').click();
    Overview.goToDashboard();
    Overview.isLoaded();
    cy.get('span.co-break-word').contains('Testing 123');
    Overview.clickNotificationDrawer();
    cy.get('.pf-c-notification-drawer__list-item-description').contains('Testing 123');
  });
})
