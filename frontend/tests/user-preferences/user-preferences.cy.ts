import { guidedTour } from '../../upstream/views/guided-tour';
import { consoleTheme, userPreferences } from '../../views/user-preferences';
import { Pages } from 'views/pages';
import { searchPage } from 'views/search';
import { listPage } from '../../upstream/views/list-page';

describe('user preferences related features', () => {
  const projectName = 'testproject-64002';
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.switchPerspective('Administrator');
  });

  after(() => {
    cy.adminCLI(`oc delete project ${projectName}`);
    userPreferences.navToGeneralUserPreferences();
    userPreferences.toggleExactMatch('disable');
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-49134,yanpzhan,UserInterface) Support dark theme for admin console', {tags: ['e2e','@osd-ccs','@rosa']}, () => {
    cy.visit('/user-preferences');
    consoleTheme.setLightTheme();
    cy.get('.pf-theme-dark').should('not.exist');
    consoleTheme.setDarkTheme();
    cy.get('.pf-theme-dark').should('exist');
    consoleTheme.setSystemDefaultTheme();
  });

  it('(OCP-64002,yapei,UserInterface) Implement strict search in console', {tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {
    cy.cliLogin();
    cy.exec(`oc new-project ${projectName}`);
    const checkAllItemsExactMatch = (word: string) => {
      cy.get('a.co-resource-item__resource-name').each(($el) => {
        const text = $el.text();
        expect(text).to.include(`${word}`);
      });
    };
    const atLeastOneResourceShown = () => {
      cy.get('[data-test-rows="resource-row"]', {timeout: 30000}).should('have.length.at.least', 1);
    };
    const atLeastOneAPIResourceShown = () => {
      cy.get('tr', {timeout: 30000}).should('have.length.at.least', 1);
    };
    const emptyResourcesFound = () => {
      cy.get('[data-test="empty-message"]').should('exist');
    };
    // verify exact match option is also available for normal users
    userPreferences.navToGeneralUserPreferences();
    userPreferences.checkExactMatchDisabledByDefault();
    Pages.gotoProjectsList();
    listPage.filter.byName('testpj');
    cy.get(`[data-test="${projectName}"]`).should('exist');

    userPreferences.navToGeneralUserPreferences();
    userPreferences.toggleExactMatch('enable');
    Pages.gotoProjectsList();
    listPage.filter.byName('testpj');
    cy.get(`[data-test="${projectName}"]`).should('not.exist');
    listPage.filter.byName('testproject');
    cy.get(`[data-test="${projectName}"]`).should('exist');

    // do more fuzzy match testings with cluster-admin user
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    userPreferences.navToGeneralUserPreferences();
    userPreferences.toggleExactMatch('disable');
    Pages.gotoProjectsList();
    listPage.filter.byName('apiver');
    atLeastOneResourceShown();

    Pages.gotoNamespacesList();
    listPage.filter.byName('conma');
    atLeastOneResourceShown();

    Pages.gotoSearch();
    searchPage.chooseResourceType('Deployment');
    searchPage.searchMethodValues('Name', 'apiver');
    atLeastOneResourceShown();

    Pages.gotoAPIExplorer();
    searchPage.searchBy('apiver');
    atLeastOneAPIResourceShown();

    Pages.gotoDeploymentsList();
    searchPage.searchBy('apiver');
    atLeastOneResourceShown();

    Pages.gotoClusterOperatorsList();
    searchPage.searchBy('conm');
    atLeastOneResourceShown();

    Pages.gotoCRDsList();
    searchPage.searchBy('apiver');
    atLeastOneResourceShown();

    // do exact match testings with cluster-admin user
    userPreferences.navToGeneralUserPreferences();
    userPreferences.toggleExactMatch('enable');
    Pages.gotoProjectsList();
    listPage.filter.byName('apiver');
    cy.get('div').contains('No results match the filter').as('no_results').should('exist');
    listPage.filter.byName('apiserver');
    cy.wait(3000);
    checkAllItemsExactMatch('apiserver');

    Pages.gotoNamespacesList();
    listPage.filter.byName('conma');
    cy.get('@no_results').should('exist');
    listPage.filter.byName('config-managed');
    cy.wait(3000);
    checkAllItemsExactMatch('config-managed');

    Pages.gotoSearch();
    searchPage.chooseResourceType('Deployment');
    searchPage.searchMethodValues('Name', 'apiver');
    emptyResourcesFound();
    searchPage.searchMethodValues('Name', 'apiserver');
    cy.wait(3000);
    checkAllItemsExactMatch('apiserver');

    Pages.gotoAPIExplorer();
    searchPage.searchBy('apiver');
    emptyResourcesFound();
    searchPage.searchBy('APIServer');
    cy.wait(3000);
    checkAllItemsExactMatch('APIServer');

    Pages.gotoDeploymentsList();
    searchPage.searchBy('apiver');
    emptyResourcesFound();
    searchPage.searchBy('apiserver');
    cy.wait(3000);
    checkAllItemsExactMatch('apiserver');

    Pages.gotoClusterOperatorsList();
    searchPage.searchBy('conm');
    emptyResourcesFound();
    searchPage.searchBy('controller-manager');
    cy.wait(3000);
    checkAllItemsExactMatch('controller-manager');

    Pages.gotoCRDsList();
    searchPage.searchBy('apiver');
    emptyResourcesFound();
    searchPage.searchBy('APIServer');
    cy.wait(3000);
    checkAllItemsExactMatch('APIServer');
  });

  it('(OCP-72562,yapei,UserInterface)Add French and Spanish language support', {tags: ['e2e','@osd-ccs','@rosa']}, () => {
    const expectedLanguages = ['English', 'Español', 'Français', '한국어', '日本語', '中文'];
    userPreferences.navToGeneralUserPreferences();
    userPreferences.getLanguageOptions()
      .then(($els) => {
        const language_list = Cypress._.map(Cypress.$.makeArray($els), 'innerText');
        expect(language_list).to.have.members(expectedLanguages);
    })
  });
})
