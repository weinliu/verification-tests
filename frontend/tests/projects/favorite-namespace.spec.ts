import { namespaceDropdown } from "views/namespace-dropdown";

describe("namespace dropdown favorite test", () => {
  before(() => {
    cy.exec(
      `oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env(
        "LOGIN_USERNAME"
      )} --kubeconfig ${Cypress.env("KUBECONFIG_PATH")}`
    );
    cy.login(
      Cypress.env("LOGIN_IDP"),
      Cypress.env("LOGIN_USERNAME"),
      Cypress.env("LOGIN_PASSWORD")
    );
  });

  after(() => {
    cy.logout();
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
  });

  it("(OCP-45319, xiangyli) - Check the system namespaces that are Favorited will list in the Favorited list even if 'show default projects' is unselectedt", {tags: ['e2e','admin']}, () => {
    const namespaces = [
      'openshift-apiserver-operator',
      'openshift-authentication-operator'
    ];

    cy.visit('/search/all-namespaces');
    namespaceDropdown.clickTheDropdown();
    namespaceDropdown.clickDefaultProjectToggle();
    namespaceDropdown.filterNamespace('openshift');

    namespaces.forEach(namespace => {
      namespaceDropdown.favoriteNamespace(namespace);
      cy.get(`li:contains(${namespace})`).should('have.length', 2);
    });

    namespaceDropdown.clickDefaultProjectToggle();
    namespaces.forEach(namespace => {
      cy.get(`li:contains(${namespace})`).should('have.length', 1);
    });
  });
});
