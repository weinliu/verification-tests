import { detailsPage } from '../../upstream/views/details-page';
import { listPage  } from '../../upstream/views/list-page';
import { logsPage } from '../../views/logs';
import { guidedTour } from '../../upstream/views/guided-tour';
import { testName } from '../../upstream/support';

describe('node logs related features', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.exec(`oc new-project ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
  });

  after(() => {
    cy.exec(`oc delete project ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-43996,yapei,UserInterface) View Master and Worker Node Logs',{tags:['@userinterface','@e2e','admin']}, () => {
    cy.log('view master node logs');
    cy.visit('/k8s/cluster/nodes?rowFilter-node-role=control-plane');
    listPage.rows.shouldBeLoaded();
    listPage.rows.clickFirstLinkInFirstRow();
    detailsPage.isLoaded();
    detailsPage.selectTab('Logs');
    logsPage.logWindowLoaded();
    // filter by Unit
    logsPage.filterByUnit('crio');
    logsPage.logLinesNotContain('hyperkube');
    // check other component audit log
    logsPage.selectLogComponent('openshift-apiserver');
    cy.contains('Select a log').should('exist');
    logsPage.selectLogFile('audit.log');
    logsPage.logWindowLoaded();

    cy.log('view worker node logs');
    cy.visit('/k8s/cluster/nodes?rowFilter-node-role=worker');
    listPage.rows.shouldBeLoaded();
    listPage.rows.clickFirstLinkInFirstRow();
    detailsPage.isLoaded();
    detailsPage.selectTab('Logs');
    logsPage.logWindowLoaded();
    // only provide filter by Unit
    logsPage.filterByUnit('systemd-journald');
    logsPage.logLinesNotContain('crio');
  });
  it('(OCP-46636,yanpzhan,UserInterface) Support for search and line number in pod/node log',{tags:['@userinterface','@e2e','admin']}, () => {
    cy.exec(`oc create -f ./fixtures/pods/pod-with-white-space-logs.yaml -n ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    //search log on node log page
    cy.visit('/k8s/cluster/nodes');
    listPage.rows.shouldBeLoaded();
    listPage.rows.clickFirstLinkInFirstRow();
    detailsPage.isLoaded();
    detailsPage.selectTab('Logs');
    logsPage.logWindowLoaded();
    logsPage.checkLogLineExist();
    logsPage.searchLog('pod');
    logsPage.clearSearch();
    logsPage.searchLog('pod');
    //search log on pod log page
    cy.visit(`/k8s/ns/${testName}/pods/example/logs`);
    logsPage.selectContainer('container2');
    logsPage.logWindowLoaded();
    logsPage.checkLogLineExist();
    cy.wait(5000);
    logsPage.searchLog('Log');
  });
})
