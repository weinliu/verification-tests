import { nav } from '../../upstream/views/nav';
import { Overview, statusCard } from '../../views/overview';
import { namespaceDropdown } from '../../views/namespace-dropdown';
import { guidedTour } from '../../upstream/views/guided-tour';
import { listPage } from '../../upstream/views/list-page';

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
    cy.exec(`oc apply -f ./fixtures/console-customization-plugin-manifest.yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    
    // login via web
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });
  beforeEach(() => {
    guidedTour.close();
    cy.intercept(
      {
        method: 'GET',
        url: '/locales/resource.json?lng=en&ns=plugin__console-demo-plugin'
      },
      {}
    ).as('getConsoleDemoPluginLocales');
    cy.intercept(
      {
        method: 'GET',
        url: '/locales/resource.json?lng=en&ns=plugin__console-customization'
      },
      {}
    ).as('getConsoleCustomizaitonPluginLocales');    
  });
  after(() => {
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"managementState":"Managed"}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc delete consoleplugin --all --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":null}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc delete namespace console-demo-plugin console-customization-plugin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
  });

  it('(OCP-54170, xiangyli) Promote ConsolePlugins API version to v1', {tags: ['e2e', 'admin']}, () => {
      cy.visit('/k8s/cluster/customresourcedefinitions/consoleplugins.console.openshift.io/instances')
      listPage.rows.shouldExist('console-demo-plugin')
      cy.exec(`oc get consoleplugin console-demo-plugin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -o yaml | grep 'apiVersion'`)
        .its('stdout')
        .should('contain', 'apiVersion: console.openshift.io/v1')        
  })

  it('(OCP-51743,yapei) Implement check for the new i18n annotation for dynamic plugins', {tags: ['e2e','admin']},() => {
    cy.log('Preload - locale files are loaded once plugin is enabled');
    // enable console-customization plugin
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":["console-customization"]}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
      .then(result => expect(result.stdout).contains('patched'));
    // Use visiting pages to refresh instead of click on 'Refresh console' button
    // which is unreliable
    cy.wait(30000);
    cy.visit('/api-explorer');
    cy.wait('@getConsoleCustomizaitonPluginLocales',{timeout: 60000});

    cy.log('Lazy - do not load locale files during enablement')
    cy.switchPerspective('Developer');
    guidedTour.close();
    // enable console-demo-plugin
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":["console-customization", "console-demo-plugin"]}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
      .then(result => expect(result.stdout).contains('patched'));
    cy.wait(30000);
    cy.visit('/topology/all-namespaces');
    cy.wait('@getConsoleDemoPluginLocales', {timeout: 60000});
    cy.on('fail', (e)=>{
      console.log(e.message)
      if (!e.message.includes('No request ever occurred')){
        throw e;
      }
    })

    cy.log('Lazy - locale files are only loaded when visit plugin pages')
    cy.switchPerspective('Developer');
    cy.clickNavLink(['Demo Plugin', 'Test Consumer']);
    cy.wait('@getConsoleDemoPluginLocales', {timeout: 30000})
      .then((intercept)=>{
        const { statusCode } = intercept.response
        expect(statusCode).to.eq(200)
      })
  });

  it('(OCP-45629,yapei) dynamic plugins proxy to services on the cluster', {tags: ['e2e','admin']},() => {
    cy.switchPerspective('Developer');
    nav.sidenav.shouldHaveNavSection(['Demo Plugin']);
    // demo plugin in Dev perspective
    cy.get('.pf-c-nav__link').should('include.text', 'Dynamic Nav 1');
    cy.get('.pf-c-nav__link').should('include.text', 'Dynamic Nav 2');
    // demo plugin in Administrator perspective
    cy.switchPerspective('Administrator');
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

  it('(OCP-50757,yapei) Support ordering of plugin nav sections in admin perspective', {tags: ['e2e','admin']}, () => {
    cy.switchPerspective('Administrator');
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

  it('(OCP-54322,yapei) Expose ErrorBoundary and improve overview detail extension', {tags: ['e2e','admin']}, () => {
    cy.log('Expose ErrorBoundary capabilities')
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

  it('(OCP-52366, xiangyli) Add Dyamic Plugins to Cluster Overview Status card and notification drawer', {tags: ['e2e','admin']}, () => {
    cy.switchPerspective('Administrator');
    Overview.goToDashboard();
    statusCard.togglePluginPopover();
    let total = 0;
    cy.exec(`oc get consoleplugin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`).then((result) => {
      total = result.stdout.split(/\r\n|\r|\n/).length - 1
    })
    cy.get(".pf-c-popover__body").within(($div) => {
      cy.get('a:contains(View all)').should('have.attr', 'href', '/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins')
      cy.contains(`${2}/${total} enabled`).should('exist')
    })
  });
  
  it('(OCP-42537,yapei) Allow disabling dynamic plugins through a query parameter', {tags: ['e2e','admin']}, () => {
    cy.switchPerspective('Administrator');
    // disable non-existing plugin will make no changes
    cy.visit('?disable-plugins=foo,bar');
    cy.get('.pf-c-nav__link',{timeout: 60000}).should('include.text','Demo Plugin');
    cy.get('.pf-c-nav__link',{timeout: 60000}).should('include.text','Customization');

    // disable one plugin
    cy.visit('?disable-plugins=console-demo-plugin')
    cy.get('.pf-c-nav__link',{timeout: 60000}).should('not.have.text','Demo Plugin');
    cy.get('.pf-c-nav__link',{timeout: 60000}).should('include.text','Customization');

    // disable all plugins
    cy.visit('?disable-plugins')
    cy.get('.pf-c-nav__link',{timeout: 60000}).should('not.have.text','Demo Plugin');
    cy.get('.pf-c-nav__link',{timeout: 60000}).should('not.have.text','Customization');
  });

  it('(OCP-53123,yapei) Exposed components in dynamic-plugin-sdk', {tags: ['e2e','admin']}, () => {
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
    nav.sidenav.shouldHaveNavSection(['Example Namespaced Page']);
    nav.sidenav.clickNavLink(['Example Namespaced Page']);
    namespaceDropdown.selectNamespace('openshift-dns');
    cy.get('h1').contains('Currently selected namespace').should('exist');
    cy.get('h2').contains('openshift-dns').should('exist');
  });

  it('(OCP-53234,yapei) Show alert when console operator is Unmanaged', {tags: ['e2e','admin']}, () => {
    // set console to Unmanaged
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"managementState":"Unmanaged"}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`).then((result) => {
      expect(result.stdout).contains('patched')
    });
    cy.visit('/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins');
    cy.get('a[data-test-id="console-demo-plugin"]').should('exist');
    cy.contains('unmanaged').should('exist');
    cy.contains('anges to plugins will have no effect').should('exist');
  })
});
