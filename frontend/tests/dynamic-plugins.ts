import { nav } from '../upstream/views/nav';
import { Overview, statusCard } from '../views/overview';
import { guidedTour } from '../upstream/views/guided-tour';

describe('Dynamic plugins features', () => {
  before(() => {
    const demoPluginNamespace = 'console-demo-plugin';
    cy.exec(`oc create namespace ${demoPluginNamespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);

    // deploy plugin manifests
    cy.exec(`oc create -f ./fixtures/demo-plugin-consoleplugin.yaml -n ${demoPluginNamespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    const resources = ['demo-plugin-deployment', 'demo-plugin-service']
    resources.forEach((resource) => {
      cy.exec(`oc create -f ./fixtures/${resource}.yaml -n ${demoPluginNamespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
        .then(result => { expect(result.stdout).contain("created")})
    });

    // enable plugin
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":["console-demo-plugin"]}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    
    // login via web
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));

    // set console to Unmanaged
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"managementState":"Unmanaged"}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`).then((result) => {
      expect(result.stdout).contains('patched')
    });
    cy.exec(`oc get cm console-config -n openshift-console -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {timeout: 60000}).then((result) => {
      expect(result.stdout).contains('console-demo-plugin')
    });
  });
  after(() => {
    cy.exec(`oc delete namespace console-demo-plugin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"managementState":"Managed"}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":null}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc delete consoleplugin console-demo-plugin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
  });
  it('(OCP-45629, admin) dynamic plugins proxy to services on the cluster', () => {
    nav.sidenav.switcher.changePerspectiveTo('Developer');
    guidedTour.close();
    cy.wait(30000);
    cy.get('body').then(($body) => {
      if ($body.find(`[data-test="toast-action"]`).length) {
        cy.contains('Refresh web console').click();
      }
    });

    // demo plugin in Dev perspective
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

  it('(OCP-52366, xiangyli) Add Dyamic Plugins to Cluster Overview Status card and notification drawer', () => {
    Overview.goToDashboard()
    statusCard.togglePluginPopover()
    let total = 0
    cy.exec(`oc get consoleplugin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`).then((result) => {
      total = result.stdout.split(/\r\n|\r|\n/).length - 1
    })
    cy.get(".pf-c-popover__body").within(($div) => {
      cy.get('a:contains(View all)').should('have.attr', 'href', '/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins')
      cy.contains(`${1}/${total} enabled`).should('exist')
    })
  })

  it('(OCP-53234,admin,yapei) Show alert when console operator is Unmanaged', () => {
    cy.visit('/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins');
    cy.get('a[data-test-id="console-demo-plugin"]').should('exist');
    cy.contains('unmanaged').should('exist');
    cy.contains('anges to plugins will have no effect').should('exist');
  })
});
