import { guidedTour } from 'views/tour';
import { DetailsPageSelector } from '../../upstream/views/details-page';
import { listPage, ListPageSelector } from '../../upstream/views/list-page';
import { userPreferences } from '../../views/user-preferences';

describe('Administration pages pesudo translation', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.switchPerspective('Administrator');
    guidedTour.close();
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-35766,yapei,UserInterface) administration pages pesudo translation',{tags:['@userinterface','@e2e','admin','@osd-ccs','@hypershift-hosted']}, () => {
    cy.log('cluster settings details pesudo translation');
    //workaround OCPBUGS-30218 Console’s pseudolocalization not working
    userPreferences.chooseDifferentLanguage('English');
    cy.visit('/settings/cluster?pseudolocalization=true&lng=en');
    cy.get('.co-cluster-settings__section', {timeout: 40000});
    cy.get('.co-cluster-settings').isPseudoLocalized();
    cy.get(DetailsPageSelector.horizontalNavTabs).isPseudoLocalized();
    cy.get(DetailsPageSelector.itemLabels).isPseudoLocalized();
    cy.get(DetailsPageSelector.sectionHeadings).isPseudoLocalized();
    cy.get('th').isPseudoLocalized();

    cy.log('cluster settings cluster operators pesudo translation');
    cy.visit('/settings/cluster/clusteroperators?pseudolocalization=true&lng=en');
    listPage.rows.shouldBeLoaded();
    cy.get('th').isPseudoLocalized();

    cy.log('cluster settings configurations pesudo translation');
    cy.visit('/settings/cluster/globalconfig?pseudolocalization=true&lng=en');
    cy.get('.co-m-table-grid', {timeout: 60000});
    cy.get('.co-help-text').isPseudoLocalized();
    cy.byLegacyTestID('item-filter').isPseudoLocalized();
    cy.get('.co-m-table-grid__head').isPseudoLocalized();

    cy.log('Namespaces list and other pages pesudo translation');
    // list page
    const test_ns = 'openshift-apiserver'
    cy.visit('/k8s/cluster/namespaces?pseudolocalization=true&lng=en');
    listPage.rows.shouldBeLoaded();
    cy.testI18n([ListPageSelector.tableColumnHeaders], ['item-create']);
    listPage.filter.byName('openshift-');
    cy.wait(5000);
    cy.byLegacyTestID('kebab-button').first().click();
    cy.get('button[data-test-action]').isPseudoLocalized();

    //details page
    cy.get('a.co-resource-item__resource-name').first().click();
    cy.get(DetailsPageSelector.horizontalNavTabs).isPseudoLocalized();
    cy.get(DetailsPageSelector.itemLabels).isPseudoLocalized();
    cy.get(DetailsPageSelector.sectionHeadings).isPseudoLocalized();
    cy.byLegacyTestID('actions-menu-button').click();
    cy.get('button[data-test-action]').isPseudoLocalized();

    // RoleBindings tab
    cy.visit(`/k8s/cluster/namespaces/${test_ns}/roles?pseudolocalization=true&lng=en`);
    listPage.rows.shouldBeLoaded();
    cy.testI18n(['th'], ['item-create']);

    // ResourceQuota and LimitRange has been covered in resource-crud.spec
    cy.log('CustomResourceDefinitions list and details pesudo translation');
    const CRD_kind_group = 'consolequickstarts.console.openshift.io';
    cy.visit('/k8s/cluster/customresourcedefinitions?pseudolocalization=true&lng=en');
    listPage.rows.shouldBeLoaded();
    cy.testI18n([ListPageSelector.tableColumnHeaders], ['item-create']);
    // CRD details
    cy.byLegacyTestID('item-filter').type('consolequickstart');
    cy.byLegacyTestID('ConsoleQuickStart').click();
    cy.get(DetailsPageSelector.horizontalNavTabs).isPseudoLocalized();
    cy.get(DetailsPageSelector.itemLabels).isPseudoLocalized();
    cy.get(DetailsPageSelector.sectionHeadings).isPseudoLocalized();
    cy.get('.co-m-table-grid__head').isPseudoLocalized();
    cy.byLegacyTestID('actions-menu-button').click();
    cy.get('button[data-test-action]').isPseudoLocalized();
    // Instances page
    cy.visit(`/k8s/cluster/customresourcedefinitions/${CRD_kind_group}/instances?pseudolocalization=true&lng=en`);
    listPage.rows.shouldBeLoaded();
    cy.testI18n(['th'], ['item-create']);
  });
})
