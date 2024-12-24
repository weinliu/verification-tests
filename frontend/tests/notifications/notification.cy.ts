import { Overview, statusCard } from '../../views/overview';
import { preferNotifications } from '../../views/user-preferences';
import testConfigMap from '../../fixtures/cluster-monitoring-config-enable-user-workload.json';
import testAlert from '../../fixtures/testalert.json';

describe('Notification drawer tests', () => {
  const user_configuration_data = 'enableUserWorkload: true';
  before(() => {
    cy.cliLogin();
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.configureClusterMonitoringConfig(user_configuration_data, testConfigMap);
    cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  afterEach(() => {
    preferNotifications.goToNotificationsTab();
    preferNotifications.toggleNotifications('hide');
  });

  after(() => {
    cy.restoreClusterMonitoringConfig();
    cy.exec('oc delete project test-ocp45305 test-ocp43119', {failOnNonZeroExit: false});
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.cliLogout();
  })

  it('(OCP-45305,yanpzhan,UserInterface) check alert on overview page and notification drawer list',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    cy.adminCLI(`oc new-project test-ocp45305`);
    cy.exec(`echo '${JSON.stringify(testAlert)}' | oc create -f - --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -n test-ocp45305`);
    preferNotifications.goToNotificationsTab();
    preferNotifications.toggleNotifications('hide');
    Overview.goToDashboard();
    Overview.isLoaded();
    cy.contains('Testing 123').should('not.exist')
    Overview.clickNotificationDrawer();
    cy.contains('[class$=notification-drawer__list-item-description]', 'Testing 123').should('not.exist');
    preferNotifications.goToNotificationsTab();
    preferNotifications.toggleNotifications('enable');
    Overview.goToDashboard();
    Overview.isLoaded();
    cy.wait(30000);
    statusCard.checkAlertItem('TestAlert', 'exist');
    Overview.clickNotificationDrawer();
    cy.get('[class$=notification-drawer__list-item-description]').contains('Testing 123');
  });

  it('(OCP-43119,yapei,UserInterface) Check alerts are filtered based on Pod/Project/Node labels',{tags:['@userinterface','@e2e','admin','@hypershift-hosted']}, () => {
    let token, query_command, query_output, scheduled_node_value;
    const alertName = 'KubePodRestartsOften';
    cy.adminCLI('oc new-project test-ocp43119');
    cy.adminCLI('oc create -f ./fixtures/pods/crash-pod.yaml -n test-ocp43119');
    cy.exec(`oc create -f ./fixtures/prometheus-rule-pod-restart.yaml -n test-ocp43119 --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
      .its('stdout')
      .should('contain', 'created');
    cy.wait(120000);

    cy.visit('/k8s/cluster/projects/test-ocp43119');
    statusCard.isLoaded();
    statusCard.checkAlertItem(`${alertName}`, 'exist');
    cy.visit('/k8s/cluster/projects/openshift-apiserver');
    statusCard.isLoaded();
    statusCard.checkAlertItem(`${alertName}`, 'not.exist');

    cy.exec('oc create token prometheus-k8s -n openshift-monitoring').then((result) => {
      token = result.stdout;
      query_command = 'oc -n openshift-monitoring exec -c prometheus prometheus-k8s-0 -- curl -k -H "Authorization: Bearer ' + token + '" \'https://alertmanager-main.openshift-monitoring.svc:9094/api/v2/alerts?&filter={alertname="KubePodRestartsOften"}\'';
    });

    cy.wrap(query_command).then(() => {
      cy.exec(`${query_command}`).then((result) => {
        query_output = result.stdout;
        const regex = /"node":".+?"/g;
        const found = query_output.match(regex);
        scheduled_node_value = found[0].split(':')[1].replace(/"/g, '')
        cy.log(`scheduled_node_value is ${scheduled_node_value}`);
        cy.visit(`/k8s/cluster/nodes/${scheduled_node_value}`);
        statusCard.isLoaded();
        statusCard.checkAlertItem(`${alertName}`, 'exist');
      });
    });

    cy.wrap(scheduled_node_value).then(() => {
      cy.exec(`oc get node --no-headers | grep -v ${scheduled_node_value} | awk '{print $1;exit}'`).then((result) => {
        const other_node_value = result.stdout;
        cy.log(`other_node_value is ${other_node_value}`);
        cy.visit(`/k8s/cluster/nodes/${other_node_value}`);
        statusCard.isLoaded();
        statusCard.checkAlertItem(`${alertName}`, 'not.exist');
      })
    });
  });
})
