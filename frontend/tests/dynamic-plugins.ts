import { DemoPluginNamespace, DemoPluginDeployment, DemoPluginService, DemoPluginConsolePlugin } from "../fixtures/demo-plugin-oc-manifests";
import { nav } from '../upstream/views/nav';
import { Overview } from '../views/overview';

describe('Dynamic plugins features', () => {
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
    cy.get('.pf-c-alert__action-group', {timeout: 240000}).within(() => {
        cy.get('button').contains('Refresh').click();
    })
  });
  after(() => {
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":null}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc delete consoleplugin console-demo-plugin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc delete namespace console-demo-plugin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.logout();
  });
  it('(OCP-45629, admin) dynamic plugins proxy to services on the cluster', () => {
    // demo plugin in Dev perspective
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

  it('(OCP-50757, admin) Support ordering of plugin nav sections in admin perspective', () => {
    nav.sidenav.switcher.changePerspectiveTo('Administrator');
    // Demo Plugin nav is rendered after Workloads, before Networking
    cy.contains('button', 'Demo Plugin').should('have.attr', 'data-test', 'nav-demo-plugin');
    cy.get('button.pf-c-nav__link')
      .eq(2)
      .should('have.text', 'Workloads');
    cy.get('button.pf-c-nav__link')
      .eq(3)
      .should('have.text', 'Demo Plugin');
    cy.get('button.pf-c-nav__link')
      .eq(4)
      .should('have.text', 'Networking');
  });
})
