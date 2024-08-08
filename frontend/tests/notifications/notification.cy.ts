import { Overview, statusCard } from '../../views/overview';
import { preferNotifications } from '../../views/user-preferences';
import { listPage } from 'upstream/views/list-page';
import testConfigMap from '../../fixtures/cluster-monitoring-config.json';
import testAlert from '../../fixtures/testalert.json';

describe('Notification drawer tests', () => {
  let $cmexisting = 0, cm_has_been_updated = false;
  before(() => {
    cy.cliLogin();
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.exec(`oc get cm cluster-monitoring-config -n openshift-monitoring --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -o json`, { failOnNonZeroExit: false }).then((result) => {
      const $ret = result.code;
      if($ret == 0){
        cy.log(`cm/cluster-monitoring-config already exist, its content is ${result.stdout}`);
        $cmexisting = 1;
        let json_outout = JSON.parse(result.stdout);
        let to_be_updated = JSON.parse(result.stdout);
        delete json_outout.metadata.uid;
        delete json_outout.metadata.resourceVersion;
        // save original data without(uid,resourceVersion) to restore
        cy.writeFile('./user-cm-restore-data.json', json_outout);
        cy.exec('cat ./user-cm-restore-data.json').then((result) => {
          cy.log(`user-cm-restore-data is ${result.stdout}`);
        });

        if(JSON.stringify(to_be_updated.data).includes('enableUserWorkload')){
          cy.log('cm/cluster-monitoring-config exist, and has uwm configurations, nothing to do');
        } else {
          cm_has_been_updated = true;
          cy.log('cm/cluster-monitoring-config exist, but no uwm configuration, adding');
          // add enableUserWorkload: true configuration and apply
          delete to_be_updated.metadata.uid;
          delete to_be_updated.metadata.resourceVersion;
          to_be_updated.data['config.yaml'] = 'enableUserWorkload: true\n' + to_be_updated.data['config.yaml'];
          cy.writeFile('./user-cm-with-uwm-data.json', to_be_updated);
          cy.exec('cat ./user-cm-with-uwm-data.json').then((result) => {
            cy.log(`user-cm-with-uwm-data is ${result.stdout}`);
          });
          cy.exec(`oc replace -f ./user-cm-with-uwm-data.json --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
            .its('stdout')
            .should('contain', 'replaced');
          cy.exec(`oc get cm cluster-monitoring-config -n openshift-monitoring --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -o yaml`).then((result) => {
            cy.log(`current cm content is ${result.stdout}`);
          })
        }
      }
      if($ret == 1){
        cy.log('cm/cluster-monitoring-config NOT exist, creating');
        cy.exec(`echo '${JSON.stringify(testConfigMap)}' | oc create -f - --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
      }
    });
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  })

  afterEach(() => {
    preferNotifications.goToNotificationsTab();
    preferNotifications.toggleNotifications('hide');
  })

  after(() => {
    cy.exec('oc delete project test-ocp45305 test-ocp43119', {failOnNonZeroExit: false});
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    if ($cmexisting == 1){
      if(cm_has_been_updated) {
        cy.adminCLI('oc replace -f ./user-cm-restore-data.json').its('stdout').should('contain', 'replaced');
      }
    }else {
      cy.adminCLI(`oc delete cm cluster-monitoring-config -n openshift-monitoring`);
    }
    cy.exec('rm ./user-cm*.json', {failOnNonZeroExit: false});
    cy.cliLogout();
  })

  it('(OCP-45305,yanpzhan,UserInterface) check alert on overview page and notification drawer list',{tags:['@userinterface','e2e','admin','@osd-ccs','@rosa']}, () => {
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

  it('(OCP-43119,yapei,UserInterface) Check alerts are filtered based on Pod/Project/Node labels',{tags:['@userinterface','e2e','admin']}, () => {
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
