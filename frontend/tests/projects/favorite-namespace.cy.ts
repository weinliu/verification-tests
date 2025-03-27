import { guidedTour } from '../../upstream/views/guided-tour';
import { namespaceDropdown } from "views/namespace-dropdown";
import { nav } from 'upstream/views/nav';

describe("namespace dropdown favorite test", () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env("LOGIN_USERNAME")}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.exec(`oc get cm -n openshift-console-user-settings --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}| grep -v user-settings-kubeadmin | grep user-settings | awk -F ' ' '{print $1}'`)
      .then(result => {
        const user_cms = result.stdout.split('\n')
        user_cms.forEach(cm => {
          cy.exec(`oc delete cm ${cm} -n openshift-console-user-settings --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        })
      })
  });

  it("(OCP-45319,xiyuzhao,UserInterface) - Check the system namespaces that are Favorited will list in the Favorited list even if 'show default projects' is unselectedt",{tags:['@userinterface','@e2e','admin','@hypershift-hosted']}, () => {
    const namespaces = [
      'openshift-apiserver-operator',
      'openshift-authentication-operator'
    ];
    nav.sidenav.clickNavLink(['Home', 'Search']);
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
