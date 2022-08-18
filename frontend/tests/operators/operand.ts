import { testName } from "upstream/support";

describe('operand tests', () => {
    before(() => {
        const fileNames = ['crd','csv1','csv2','csv3','operand']
        cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`oc new-project ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.wrap(fileNames).each(fileName => {
            cy.exec(`oc create -f ./fixtures/${fileName}.yaml -n ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
              .then(result => { expect(result.stdout).contain("created")
            })
        });  
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));        
    });

    after(() => {
        cy.exec(`oc delete project ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`oc delete customresourcedefinition mock-resources.test.tectonic.com --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.logout();
    });

    it('(OCP-46583,xiyuzhao) Operator should be able to customize order of conditions table', () => {
        // If a descriptor is defined on the status.conditions property, 
        // It will be rendered in the order it appears in the CSV descriptors array
        cy.visit(`/k8s/ns/${testName}/clusterserviceversions/mock-operator/test.tectonic.com~v1~MockResource/mock-resource-instance`)
          .get('[data-test-id="resource-title"]')
          .should("contain.text","mock");
        cy.wait(5000);        
        cy.get(".co-section-heading span").should('have.length',4)
          .and(($span) => { 
            expect($span.get(1).textContent, 'first section').to.equal('Custom Conditions')
            expect($span.get(2).textContent, 'second section').to.equal('Conditions')
            expect($span.get(3).textContent, 'third section').to.equal('Other Custom Conditions')
           });

        // If no x-descriptor is defined on the status.conditions property, it will be rendered as the first conditions table
        // If custome defined conditions did not have x-descriptor property, it will not be shown (eg: Other Custom Conditions)
        cy.visit(`/k8s/ns/${testName}/clusterserviceversions/mock-operator2/test.tectonic.com~v1~MockResource/mock-resource-instance`)
          .get('[data-test-id="resource-title"]')
          .should("contain.text","mock");
        cy.wait(5000);        
        cy.get(".co-section-heading span").should('have.length',3)
          .and(($span) => { 
            expect($span.get(1).textContent, 'first section').to.equal('Conditions')
            expect($span.get(2).textContent, 'second section').to.equal('Custom Conditions')
           });

        //If no default status.conditions property is being set
        //it will be rendered as the first conditions table, and then follow the defined order by custome
        cy.visit(`/k8s/ns/${testName}/clusterserviceversions/mock-operator3/test.tectonic.com~v1~MockResource/mock-resource-instance`)
          .get('[data-test-id="resource-title"]')
          .should("contain.text","mock");
        cy.wait(5000);        
        cy.get(".co-section-heading span").should('have.length',4)
          .and(($span) => { 
            expect($span.get(1).textContent, 'first section').to.equal('Conditions')
            expect($span.get(2).textContent, 'second section').to.equal('Other Custom Conditions')
            expect($span.get(3).textContent, 'third section').to.equal('Custom Conditions')
           });
    });
}) 