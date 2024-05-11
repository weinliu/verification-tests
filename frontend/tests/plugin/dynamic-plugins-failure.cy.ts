import { Overview, statusCard } from '../../views/overview';
import { ClusterSettingPage } from '../../views/cluster-setting';

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
    cy.adminCLI(`oc patch console.operator cluster --type='json' -p='[{"op": "add", "path": "/spec/plugins/-", "value":"${testParams.failPluginName}"}]'`);
    cy.adminCLI(`oc patch console.operator cluster --type='json' -p='[{"op": "add", "path": "/spec/plugins/-", "value":"${testParams.pendingPluginName}"}]'`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  })

  after(() => {
    ClusterSettingPage.goToConsolePlugins();
    ClusterSettingPage.toggleConsolePlugin(`${testParams.failPluginName}`, 'Disable');
    ClusterSettingPage.toggleConsolePlugin(`${testParams.pendingPluginName}`, 'Disable');
    cy.adminCLI(`oc delete consoleplugin ${testParams.failPluginName} ${testParams.pendingPluginName}`,{failOnNonZeroExit: false});
    cy.adminCLI(`oc delete namespace ${testParams.failPluginNamespace} ${testParams.pendingPluginNamespace}`,{timeout: 1200000,failOnNonZeroExit: false});
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`,{failOnNonZeroExit: false});
  })

  it('(OCP-55427,yapei,UserInterface) Improve information for Pending or Failed plugins', {tags: ['e2e', 'admin','@osd-ccs']}, () => {
    cy.adminCLI(`oc get console.operator cluster -o jsonpath='{.spec.plugins}'`)
      .its('stdout')
      .should('include', `${testParams.failPluginName}`)
      .and('include',`${testParams.pendingPluginName}`)
    // wait 60000ms then reload console pages to load all enabled plugins
    cy.wait(60000);
    cy.visit('/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins');
    cy.get('tr').should('exist');
    cy.get('[data-label="Status"]').should('exist');
    checkStatusMessage('Failed', 'ailed to get a valid plugin manifest');

    cy.log('Check failed status on Cluster Overview and notification drawer')
    Overview.goToDashboard();
    Overview.isLoaded();
    statusCard.secondaryStatus('Dynamic Plugins', 'Degraded');
    statusCard.toggleItemPopover("Dynamic Plugins");
    let total = 0;
    cy.adminCLI(`oc get consoleplugin`).then((result) => {
      total = result.stdout.split(/\r\n|\r|\n/).length - 1
    })
    cy.get('[class*="popover__body"]').within(($div) => {
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
