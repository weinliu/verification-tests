import { checkErrors } from '../upstream/support';
import { detailsPage } from '../upstream/views/details-page';
import { listPage  } from '../upstream/views/list-page';
import { logsPage } from '../views/logs';

describe('Access to Node logs', () => {
    before(() => {
        cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    });

    after(() => {
        cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.logout;
    });

    it('(OCP-43996,admin) View Master Node Logs', () => {
        cy.visit('/k8s/cluster/nodes?rowFilter-node-role=master');
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
    });
    it('(OCP-43996,admin) View Worker Node logs', () => {
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
})