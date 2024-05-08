import { nav } from '../../upstream/views/nav';
import { Overview, statusCard } from '../../views/overview';
import { namespaceDropdown } from '../../views/namespace-dropdown';
import { Branding } from '../../views/branding';
import { ClusterSettingPage } from '../../views/cluster-setting';
import { guidedTour } from '../../upstream/views/guided-tour';
import { listPage } from '../../upstream/views/list-page';

describe('Dynamic plugins features', () => {
  before(() => {
    const query_console_dmeo_plugin_pod = `oc get deployment console-demo-plugin -n console-demo-plugin -o jsonpath='{.status.conditions[?(@.type=="Available")].status}'`;
    // deploy plugin manifests
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI('oc apply -f ./fixtures/console-customization-plugin-manifests.yaml');
    cy.adminCLI('oc apply -f ./fixtures/console-demo-plugin-manifests.yaml');
    cy.checkCommandResult(query_console_dmeo_plugin_pod, 'True', { retries: 3, interval: 15000 }).then(() => {
      cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
      return;
    });
  });

  beforeEach(() => {
    guidedTour.close();
  });

  after(() => {
    cy.adminCLI(`oc patch console.operator cluster -p '{"spec":{"managementState":"Managed"}}' --type merge`);
    ClusterSettingPage.goToConsolePlugins();
    ClusterSettingPage.toggleConsolePlugin('console-customization', 'Disable');
    ClusterSettingPage.toggleConsolePlugin('console-demo-plugin', 'Disable');
    cy.adminCLI(`oc delete namespace console-demo-plugin console-customization-plugin`,{failOnNonZeroExit: false});
    cy.adminCLI(`oc delete consoleplugin console-customization console-demo-plugin`,{failOnNonZeroExit: false});
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`,{failOnNonZeroExit: false});
  });

  it('(OCP-51743,yapei,UserInterface)Preload - locale files are loaded once plugin is enabled', {tags: ['e2e','admin','@osd-ccs']}, () => {
    cy.intercept(
      {
        method: 'GET',
        url: '/locales/resource.json?lng=en&ns=plugin__console-customization'
      },
      {}
    ).as('getConsoleCustomizaitonPluginLocales');
    cy.log('Preload - locale files are loaded once plugin is enabled');
    // enable console-customization plugin
    cy.adminCLI(`oc patch console.operator cluster --type='json' -p='[{"op": "add", "path": "/spec/plugins/-", "value":"console-customization"}]'`)
      .then(result => expect(result.stdout).contains('patched'));
    // Use visiting pages to refresh instead of click on 'Refresh console' button
    // which is unreliable
    cy.adminCLI(`oc get console.operator cluster -o jsonpath='{.spec.plugins}'`).then((result) => {
      expect(result.stdout).contains('console-customization')
    });
    cy.wait(30000);
    cy.visit('/api-explorer');
    cy.wait('@getConsoleCustomizaitonPluginLocales',{timeout: 60000});
  });

  it('(OCP-51743,yapei,UserInterface)Lazy - do not load locale files during enablement',{tags: ['e2e','admin','@osd-ccs']},() => {
    cy.intercept(
      {
        method: 'GET',
        url: '/locales/resource.json?lng=en&ns=plugin__console-demo-plugin'
      },
      {}
    ).as('getConsoleDemoPluginLocales');
    cy.switchPerspective('Developer');
    guidedTour.close();
    // enable console-demo-plugin
    cy.adminCLI(`oc patch console.operator cluster --type='json' -p='[{"op": "add", "path": "/spec/plugins/-", "value":"console-demo-plugin"}]'`)
      .then(result => expect(result.stdout).contains('patched'));
    cy.adminCLI(`oc get console.operator cluster -o jsonpath='{.spec.plugins}'`).then((result) => {
      expect(result.stdout).contains('console-customization').and.contains('console-demo-plugin');
    });
    cy.wait(30000);
    cy.visit('/topology/all-namespaces');
    cy.wait('@getConsoleDemoPluginLocales', {timeout: 60000});
    cy.on('fail', (e)=>{
      console.log(e.message)
      if (!e.message.includes('No request ever occurred')){
        throw e;
      }
    });

    cy.log('Lazy - locale files are only loaded when visit plugin pages')
    cy.switchPerspective('Developer');
    cy.clickNavLink(['Demo Plugin', 'Test Consumer']);
    cy.wait('@getConsoleDemoPluginLocales', {timeout: 30000})
      .then((intercept)=>{
        const { statusCode } = intercept.response
        expect(statusCode).to.eq(200)
      })
  });

  it('(OCP-50757,yapei,UserInterface) Support ordering of plugin nav sections in admin perspective', {tags: ['e2e','admin','@osd-ccs']}, () => {
    cy.switchPerspective('Administrator');
    // Demo Plugin nav is rendered after Workloads, before Networking
    cy.contains('button', 'Demo Plugin').should('have.attr', 'data-test', 'nav-demo-plugin');
    cy.get('button.pf-v5-c-nav__link')
      .then(($els) => {
        const original_array = Cypress._.map(Cypress.$.makeArray($els), 'innerText');
        const filtered_array = original_array.filter((word) => word ==='Workloads' || word === 'Demo Plugin' || word === 'Networking')
        return filtered_array;
      })
      .should('be.an', 'array')
      .and('have.ordered.members', ['Workloads', 'Demo Plugin', 'Networking']);
  });

  it('(OCP-54322,yapei,UserInterface) Expose ErrorBoundary and improve overview detail extension', {tags: ['e2e','admin','@osd-ccs']}, () => {
    cy.log('Expose ErrorBoundary capabilities');
    cy.switchPerspective('Administrator');
    cy.visit('/sample-error-boundary-page');
    cy.contains('Launch buggy component').click({force: true});
    cy.contains('Show details').click({force: true});
    cy.contains('something went wrong in your dynamic plug-in').should('exist');
    cy.contains('test error').should('exist');

    cy.log('Improve overview detail item extension')
    cy.switchPerspective('Administrator');
    Overview.goToDashboard();
    cy.get('[data-test="detail-item-title"]').should('include.text','Custom Overview Detail Title');
    cy.get('[data-test="detail-item-value"]').should('include.text','Custom Overview Detail Info');
  });

  it('(OCP-52366,yapei,UserInterface) Add Dyamic Plugins to Cluster Overview Status card and notification drawer', {tags: ['e2e','admin','@osd-ccs']}, () => {
    cy.switchPerspective('Administrator');
    Overview.goToDashboard();
    statusCard.toggleItemPopover("Dynamic Plugins");
    let enabled = 0
    cy.window().its('SERVER_FLAGS.consolePlugins').then((result) => {
      enabled = result.length
    })
    let total = 0;
    cy.adminCLI(`oc get consoleplugin`).then((result) => {
      total = result.stdout.split(/\r\n|\r|\n/).length - 1
    })
    cy.get(".pf-v5-c-popover").within(($div) => {
      cy.get('a:contains(View all)').should('have.attr', 'href', '/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins')
      cy.contains(`${enabled}/${total} enabled`).should('exist')
    })
  });

  it('(OCP-56239,yapei,UserInterface) Add dynamic plugin info to About modal', {tags: ['e2e', 'admin','@osd-ccs']}, () => {
    cy.switchPerspective('Administrator')
    Overview.toggleAbout()
    cy.contains('Dynamic plugins').should('exist')
    cy.contains('console-demo-plugin (0.0.0)').should('exist')
    cy.contains('console-customization (0.0.1)').should('exist')
    Branding.closeModal()
  });

  it('(OCP-42537,yapei,UserInterface) Allow disabling dynamic plugins through a query parameter', {tags: ['e2e','admin','@osd-ccs']}, () => {
    Branding.closeModal()
    cy.switchPerspective('Administrator');
    // disable non-existing plugin will make no changes
    cy.visit('?disable-plugins=foo,bar');
    cy.get('a[data-test="nav"]',{timeout: 60000}).should('include.text','Dynamic Nav');
    cy.get('a[data-test="nav"]',{timeout: 60000}).should('include.text','Customization');

    // disable one plugin
    cy.visit('?disable-plugins=console-demo-plugin')
    cy.get('a[data-test="nav"]',{timeout: 60000}).should('not.have.text','Dynamic Nav');
    cy.get('a[data-test="nav"]',{timeout: 60000}).should('include.text','Customization');

    // disable all plugins
    cy.visit('?disable-plugins')
    cy.get('a[data-test="nav"]',{timeout: 60000}).should('not.have.text','Dynamic Nav');
    cy.get('a[data-test="nav"]',{timeout: 60000}).should('not.have.text','Customization');
    cy.visit('/api-explorer');
  });

  it('(OCP-45629,yapei,UserInterface) dynamic plugins proxy to services on the cluster', {tags: ['e2e','admin','@osd-ccs']},() => {
    cy.switchPerspective('Developer');
    nav.sidenav.clickNavLink(['Demo Plugin']);
    // demo plugin in Dev perspective
    cy.get('a[data-test="nav"]').should('include.text', 'Dynamic Nav 1');
    cy.get('a[data-test="nav"]').should('include.text', 'Dynamic Nav 2');
    // demo plugin in Demo Plugin perspective
    nav.sidenav.switcher.changePerspectiveTo('Demo');
    cy.get('a[data-test="nav"]').should('include.text', 'Dynamic Nav 1');
    cy.get('a[data-test="nav"]').should('include.text', 'Dynamic Nav 2');
    // demo plugin in Administrator perspective
    cy.switchPerspective('Administrator');
    nav.sidenav.clickNavLink(['Demo Plugin']);
    cy.get('a[data-test="nav"]').should('include.text', 'Dynamic Nav 1');
    cy.get('a[data-test="nav"]').should('include.text', 'Dynamic Nav 2');
    cy.visit('/test-proxy-service');
    cy.contains('success').should('be.visible');
  });

  it('(OCP-53123,yapei,UserInterface) Exposed components in dynamic-plugin-sdk', {tags: ['e2e','admin','@osd-ccs']}, () => {
    // ResourceIcon is exposed
    cy.switchPerspective('Administrator');
    cy.visit('/demo-list-page');
    cy.get('table').should('exist');
    cy.contains('Sample ResourceIcon').should('exist');
    cy.get('[title="Pod"]').should('exist');

    // Modal is exposed
    cy.visit('/test-modal');
    cy.contains('Launch Modal').click({force: true});
    cy.get('[role="dialog"]').should('be.visible');
    cy.get('button[aria-label="Close"]').as('closebutton').click();
    cy.contains('Launch Modal Asynchronously').click({force: true});
    cy.get('[role="dialog"]').should('be.visible');
    cy.get('@closebutton').click();

    // NamespaceBar is exposed
    cy.switchPerspective('Demo');
    nav.sidenav.clickNavLink(['Example Namespaced Page']);
    namespaceDropdown.selectNamespace('openshift-dns');
    cy.get('h1').contains('Currently selected namespace').should('exist');
    cy.get('h2').contains('openshift-dns').should('exist');
  });

  it('(OCP-41459,yapei,UserInterface)Add support for analytics and integration with Segment', {tags: ['e2e','admin','@osd-ccs']}, () => {
    cy.visit('/k8s/ns/default/core~v1~Secret', {
      onBeforeLoad (win) {
        cy.spy(win.console, 'log').as('console.log')
      }
    });
    cy.switchPerspective('Administrator');
    cy.get('@console.log').should('be.calledWith', "Demo Plugin received telemetry event: ", "page");
    cy.get('@console.log').should('be.calledWith', "Demo Plugin received telemetry event: ", "Perspective Changed");
    cy.get('@console.log').should('be.calledWith', "Demo Plugin received telemetry event: ", "identify");
  });

  it('(OCP-54170,yapei,UserInterface) Promote ConsolePlugins API version to v1', {tags: ['e2e', 'admin','@osd-ccs']}, () => {
    cy.visit('/k8s/cluster/customresourcedefinitions/consoleplugins.console.openshift.io/instances')
    listPage.rows.shouldExist('console-demo-plugin')
    cy.exec(`oc get consoleplugin console-demo-plugin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -o yaml | grep 'apiVersion'`)
      .its('stdout')
      .should('contain', 'apiVersion: console.openshift.io/v1')
})

  it('(OCP-53234,yapei,UserInterface) Show alert when console operator is Unmanaged', {tags: ['e2e','admin','@osd-ccs']}, () => {
    // set console to Unmanaged
    cy.adminCLI(`oc patch console.operator cluster -p '{"spec":{"managementState":"Unmanaged"}}' --type merge`).then((result) => {
      expect(result.stdout).contains('patched')
    });
    cy.visit('/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins');
    cy.get('a[data-test-id="console-demo-plugin"]').should('exist');
    cy.contains('unmanaged').should('exist');
    cy.contains('anges to plugins will have no effect').should('exist');
  })
});
