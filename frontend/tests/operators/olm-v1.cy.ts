import { listPage } from "upstream/views/list-page";
import { nav } from '../../upstream/views/nav';
import { catalog } from 'views/catalogs';
import { Pages } from 'views/pages';

describe('OLM V1 tests', () => {
  const login_user = Cypress.env('LOGIN_USERNAME');
  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${login_user}`,{failOnNonZeroExit: false});
    cy.adminCLI(`oc delete project ns-75036`,{failOnNonZeroExit: false});
    cy.adminCLI(`oc delete clustercatalog ui-auto-custom-operators`,{failOnNonZeroExit: false});
    cy.adminCLI(`oc delete clusterextension test-ce-75036`,{failOnNonZeroExit: false});
    cy.adminCLI(`oc delete clusterrole ns-75036-sa75036`,{failOnNonZeroExit: false});
    cy.adminCLI(`oc delete clusterrolebinding ns-75036-sa75036`,{failOnNonZeroExit: false});
  })

  it('(OCP-75036,yapei,UserInterface)List packages from catalogs',{tags:['@userinterface','@e2e','admin']}, function (){
    cy.isTechPreviewNoUpgradeEnabled().then(value => {
      if (value === false) {
        cy.log('Skip the case because TP not enabled!!');
        this.skip();
      }
    });
    const test_ns = 'ns-75036';
    const cluster_extension_name = 'test-ce-75036';
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${login_user}`);
    cy.adminCLI(`oc create -f ./fixtures/operators/cluster-catalog.yaml`);
    cy.adminCLI(`oc new-project ${test_ns}`);
    cy.login(Cypress.env('LOGIN_IDP'), login_user, Cypress.env('LOGIN_PASSWORD'));
    cy.adminCLI(`oc create -f ./fixtures/operators/cluster-extension-sa-rb.yaml -f ./fixtures/operators/cluster-extension.yaml`);
    nav.sidenav.clickNavLink(['Ecosystem', 'Extension Catalog']);
    catalog.extensionCatalogLoaded();
    Pages.gotoInstalledExtensionsPage();
    listPage.rows.shouldBeLoaded();
    cy.byTestID(`${cluster_extension_name}`).click();
    cy.get('[data-test-section-heading="Conditions"]').scrollIntoView();
    cy.get('span').contains('unpack successful').should('exist');
  });
});