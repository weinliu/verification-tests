import {podsPage} from "../views/pods";
import {namespaceDropdown} from "../views/namespace-dropdown";


describe('Projects dropdown tests', () => {
    before(() => {
        cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    });

    after(() => {
        cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.logout;
    });

    it('(OCP-43130) Check default projects toggle bar', {tags: ['e2e','admin']}, () => {
        // podsPage.goToPodsInAllNamespaces()
        podsPage.goToPodsForGivenNamespace('openshift-apiserver')
        namespaceDropdown.clickTheDropdown()
        namespaceDropdown.getProjectsDisplayed()
            .each(($el, index, $list) => {
                cy.wrap($el).should('not.have.text', 'default')
                cy.wrap($el).should('not.have.text', 'kube')
                cy.wrap($el).should('not.have.text', 'openshift')
            })
        namespaceDropdown.clickDefaultProjectToggle()
        namespaceDropdown.getProjectsDisplayed().then(($els) => {
            const texts = Array.from($els, el => el.innerText);
            expect(texts.toString()).to.have.string('default')
            expect(texts.toString()).to.have.string('kube')
            expect(texts.toString()).to.have.string('openshift')
        });
        namespaceDropdown.clickDefaultProjectToggle()
    });
})
