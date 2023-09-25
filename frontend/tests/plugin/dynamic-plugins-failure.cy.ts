import { Overview, statusCard } from '../../views/overview';

describe('Dynamic Plugins notification features', () => {
  const testParams = {
    failPluginName: 'console-customization',
    failPluginNamespace: 'console-customization-plugin',
    failPluginFileName: 'failed-console-customization-plugin.yaml',
    pendingPluginName: 'console-demo-plugin-1',
    pendingPluginNamespace: 'console-demo-plugin-1',
    pendingPluginFileName: 'pending-console-demo-plugin-1.yaml'
  };

  let checkStatusMessage = (status: string, message: string) => {
    cy.get('[data-test="status-text"]', {timeout: 30000}).contains(`${status}`).click();
    cy.contains(`${message}`).should('be.visible');
  };

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc create -f ./fixtures/plugin/${testParams.failPluginFileName}`);
    cy.adminCLI(`oc create -f ./fixtures/plugin/${testParams.pendingPluginFileName}`);
    cy.adminCLI(`oc patch console.operator cluster -p '{"spec":{"plugins":["${testParams.failPluginName}", "${testParams.pendingPluginName}"]}}' --type merge`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  })

  after(() => {
    cy.adminCLI(`oc patch console.operator cluster -p '{"spec":{"plugins":null}}' --type merge`);
    cy.adminCLI(`oc delete consoleplugin ${testParams.failPluginName} ${testParams.pendingPluginName}`);
    cy.adminCLI(`oc delete namespace ${testParams.failPluginNamespace} ${testParams.pendingPluginNamespace}`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  })

  it('(OCP-55427,yapei) Improve information for Pending or Failed plugins', {tags: ['e2e', 'admin','@osd-ccs']}, () => {
    cy.adminCLI(`oc get cm console-config -n openshift-console -o yaml`)
      .its('stdout')
      .should('include', 'console-customization')
      .and('include','console-demo-plugin-1')
    cy.visit('/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins');
    cy.get('[data-label="Status"]').should('exist');
    cy.visit('/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins');
    checkStatusMessage('Failed', 'ailed to get a valid plugin manifest');
  });

  it('(OCP-52366, yapei) ocp52366-failure add Dyamic Plugins to Cluster Overview Status card and notification drawer', {tags: ['e2e','admin']}, () => {
    Overview.goToDashboard();
    Overview.isLoaded();
    statusCard.secondaryStatus('Dynamic Plugins', 'Degraded');
    statusCard.toggleItemPopover("Dynamic Plugins");
    let total = 0;
    cy.adminCLI(`oc get consoleplugin`).then((result) => {
      total = result.stdout.split(/\r\n|\r|\n/).length - 1
    })
    cy.get(".pf-c-popover__body").within(($div) => {
      cy.get('a:contains(View all)').should('have.attr', 'href', '/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins')
      cy.get(`.text-secondary`).should(($element) => {
        const text = $element.text();
        const regrex = new RegExp(`^(0|1)\/${total} enabled$`);
        expect(text).to.match(regrex);
      })
      // cy.contains(`${enabled}/${total} enabled`).should('exist')
      cy.contains('Failed plugins').should('exist')
    });
    Overview.clickNotificationDrawer();
    cy.contains('Dynamic plugin error').should('exist');
    cy.byButtonText('View plugin').click();
    cy.byLegacyTestID('resource-title').contains(testParams.failPluginName);
  });

})
