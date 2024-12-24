import {podsPage} from "../../views/pods";
import {namespaceDropdown} from "../../views/namespace-dropdown";


describe('Projects dropdown tests', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-43130,yapei,UserInterface) Check default projects toggle bar',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa','@smoke','@hypershift-hosted']}, () => {
    // podsPage.goToPodsInAllNamespaces()
    podsPage.goToPodsForGivenNamespace('openshift-apiserver')
    namespaceDropdown.clickTheDropdown()
    namespaceDropdown.getProjectsDisplayed().each(($el, index, $list) => {
      cy.wrap($el).should('not.have.text', 'default')
      cy.wrap($el).should('not.have.text', 'kube')
      cy.wrap($el).should('not.have.text', 'openshift')
    })
    namespaceDropdown.showSystemProjects()
    namespaceDropdown.getProjectsDisplayed().then(($els) => {
      const texts = Array.from($els, el => el.innerText);
      expect(texts.toString()).to.have.string('default')
      expect(texts.toString()).to.have.string('kube')
      expect(texts.toString()).to.have.string('openshift')
    });
    namespaceDropdown.hideSystemProjects()
  });
})
