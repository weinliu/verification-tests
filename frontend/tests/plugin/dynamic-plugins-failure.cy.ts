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

  let enabled = 0;
  let total = 2;
  let checkStatusMessage = (status: string, message: string) => {
    cy.get('button').contains(`${status}`).click();
    cy.contains(`${message}`).should('be.visible');
  };

  before(() => {
    cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc create -f ./fixtures/plugin/${testParams.failPluginFileName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc create -f ./fixtures/plugin/${testParams.pendingPluginFileName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":["${testParams.failPluginName}", "${testParams.pendingPluginName}"]}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  })

  after(() => {
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":null}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc delete consoleplugin ${testParams.failPluginName} ${testParams.pendingPluginName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc delete namespace ${testParams.failPluginNamespace} ${testParams.pendingPluginNamespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
  })

  it('(OCP-55427,yapei) Improve information for Pending or Failed plugins', {tags: ['e2e', 'admin']}, () => {
    cy.exec(`oc get cm console-config -n openshift-console -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
      .its('stdout')
      .should('include', 'console-customization')
      .and('include','console-demo-plugin-1')
    cy.visit('/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins');
    cy.get('[data-label="Status"]').should('exist');
    checkStatusMessage('Failed', 'ailed to get a valid plugin manifest');
  });

  it('(OCP-52366, xiangyli) ocp52366-failure add Dyamic Plugins to Cluster Overview Status card and notification drawer', {tags: ['e2e','admin']}, () => {
    Overview.goToDashboard();
    Overview.isLoaded();
    statusCard.secondaryStatus('Dynamic Plugins', 'Degraded');
    statusCard.toggleItemPopover("Dynamic Plugins");
    cy.get(".pf-c-popover__body").within(($div) => {
      cy.get('a:contains(View all)').should('have.attr', 'href', '/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins')
      cy.contains(`${enabled}/${total} enabled`).should('exist')
      cy.contains('Failed plugins').should('exist')
    });
    Overview.clickNotificationDrawer();
    cy.contains('Dynamic plugin error').should('exist');
    cy.byButtonText('View plugin').click();
    cy.byLegacyTestID('resource-title').contains(testParams.failPluginName);
  });

})
