import { DemoPluginNamespace, DemoPluginDeployment, DemoPluginService, DemoPluginConsolePlugin } from "../fixtures/demo-plugin-oc-manifests";
import { checkErrors } from '../upstream/support';
import { nav } from '../upstream/views/nav';
import { Overview } from '../views/overview';

describe('Allow dynamic plugins to proxy to services on the cluster (OCP-45629, admin)', () => {
    before(() => {
        // deploy plugin manifests
        cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`echo '${JSON.stringify(DemoPluginNamespace)}' | oc create -f - --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`echo '${JSON.stringify(DemoPluginService)}' | oc create -f - -n ${JSON.stringify(DemoPluginNamespace.metadata.name)} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        var json = `${JSON.stringify(DemoPluginDeployment)}`;
        var obj = JSON.parse(json, (k, v) => k == 'image' && /PLUGIN_IMAGE/.test(v) ? 'quay.io/openshifttest/console-demo-plugin:411' : v);
        cy.exec(`echo '${JSON.stringify(obj)}' | oc create -f - -n ${JSON.stringify(DemoPluginNamespace.metadata.name)} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`echo '${JSON.stringify(DemoPluginConsolePlugin)}' | oc create -f - --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        // enable plugin
        cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":["console-demo-plugin"]}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        // login via web
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    });
    after(() => {
        checkErrors();
        cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":null}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`oc delete consoleplugin console-demo-plugin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`oc delete namespace console-demo-plugin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.logout;
    });
    it('dynamic plugins proxy to services on the cluster', () => {
        // demo plugin in Dev perspective
        cy.get('.pf-c-alert__action-group', {timeout: 240000}).within(() => {
            cy.get('button').contains('Refresh').click();
        })
        Overview.isLoaded();
        nav.sidenav.clickNavLink(['Demo Plugin']);
        nav.sidenav.shouldHaveNavSection(['Demo Plugin']);
        cy.get('.pf-c-nav__link').should('include.text', 'Dynamic Nav 1');
        cy.get('.pf-c-nav__link').should('include.text', 'Dynamic Nav 2');
        // demo plugin in Administrator perspective
        nav.sidenav.switcher.changePerspectiveTo('Administrator');
        nav.sidenav.clickNavLink(['Demo Plugin']);
        cy.get('.pf-c-nav__link').should('include.text', 'Dynamic Nav 1');
        cy.get('.pf-c-nav__link').should('include.text', 'Dynamic Nav 2');
        // demo plugin in Demo Plugin perspective
        nav.sidenav.switcher.changePerspectiveTo('Demo');
        cy.get('.pf-c-nav__link').should('include.text', 'Dynamic Nav 1');
        cy.get('.pf-c-nav__link').should('include.text', 'Dynamic Nav 2');
        cy.visit('/test-proxy-service');
        cy.contains('success').should('be.visible');

    });
})
