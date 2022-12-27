import { testName } from "upstream/support";
import { guidedTour } from "upstream/views/guided-tour";

describe('pod page', () => {
    before(() => {
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));        
        guidedTour.close();
        cy.createProject(testName);
        cy.exec(`oc create -f ./fixtures/poddefault.yaml -n ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    });

    after(() => {
        cy.exec(`oc delete project ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    });

    it('(OCP-53357,xiyuzhao) Pod host IP is visible on Pod details page', {tags: ['e2e']}, () => {
        cy.visit(`/k8s/ns/${testName}/pods/example`);
        cy.get('[data-test="Host IP"]')
          .should('exist')
          .click()
          .should('contain.text', "Host IP");
        cy.get('[data-test-selector="details-item-value__Host IP"]')
          .should('be.visible')
          .then($a => {
            const podHostIP = $a.text();
            cy.exec(`oc get pod example -n ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -o yaml | awk '/hostIP: / {print $2}'`, { failOnNonZeroExit: false })
              .then((output) => {
                expect(output.stdout).to.equal(podHostIP);
          });
        })
    });
}) 