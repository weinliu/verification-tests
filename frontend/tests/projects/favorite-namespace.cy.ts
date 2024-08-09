import { guidedTour } from '../../upstream/views/guided-tour';
import { namespaceDropdown } from "views/namespace-dropdown";

describe("namespace dropdown favorite test", () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env("LOGIN_USERNAME")}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it("(OCP-45319,xiyuzhao,UserInterface) - Check the system namespaces that are Favorited will list in the Favorited list even if 'show default projects' is unselectedt",{tags:['@userinterface','@e2e','admin']}, () => {
    const namespaces = [
      'openshift-apiserver-operator',
      'openshift-authentication-operator'
    ];

    cy.visit('/search/all-namespaces');
    namespaceDropdown.clickTheDropdown();
    namespaceDropdown.showSystemProjects();
    namespaces.forEach(namespace => {
      namespaceDropdown.filterNamespace(namespace);
      namespaceDropdown.addFavoriteNamespace(namespace);
      cy.get(`li:contains(${namespace})`).should('have.length', 2);
    });
    /*  Starred system namespace will list in dropdown
        Even if 'Show default project' is disable */
    cy.visit('/k8s/all-namespaces/core~v1~Pod');
    namespaceDropdown.clickTheDropdown();
    namespaceDropdown.hideSystemProjects();
    namespaces.forEach(namespace => {
      cy.get(`li:contains(${namespace})`).should('have.length', 1);
      namespaceDropdown.removeFavoriteNamespace(namespace);
    });
  });
});
