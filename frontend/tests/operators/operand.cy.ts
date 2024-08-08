import { testName } from "upstream/support";
import { Operand } from "../../views/operator-hub-page";

describe('operand tests', () => {
  before(() => {
    const fileNames = ['crd','csv1','csv2','csv3','operand']
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc new-project ${testName}`);
    cy.wrap(fileNames).each(fileName => {
      cy.adminCLI(`oc create -f ./fixtures/${fileName}.yaml -n ${testName}`)
        .then(result => { expect(result.stdout).contain("created")
      })
    });
    cy.cliLogin();
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc delete project ${testName}`);
    cy.adminCLI(`oc delete customresourcedefinition mock-resources.test.tectonic.com`);
    cy.cliLogout();
  });

  it('(OCP-46583,xiyuzhao,UserInterface) Operator should be able to customize order of conditions table',{tags:['@userinterface','e2e','admin','@osd-ccs','@rosa']}, () => {
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

  it('(OCP-63078,yapei,UserInterface)Enable filtering for k8sResourcePrefix x-descriptor',{tags:['@userinterface','e2e','admin','@osd-ccs','@rosa']}, () => {
    // create several secrets with different labels for later filtering
    let secret = [{'name': 'test-secret-equity', 'literal': 'key111=value111', 'labels': 'test=true level=staging'},
                  {'name': 'test-secret-set', 'literal': 'key222=value222', 'labels': 'test=true level=production'},
                  {'name': 'test-secret-mix', 'literal': 'key333=value333', 'labels': 'level=qa'},
                  {'name': 'test-secret-all', 'literal': 'key444=value444', 'labels': 'all=true'}]
    cy.wrap(secret).each((item) => {
      cy.adminCLI(`oc create secret generic ${item.name} --from-literal=${item.literal} -n ${testName}`);
      cy.adminCLI(`oc label secret ${item.name} ${item.labels} -n ${testName}`);
    });
    cy.exec(`oc get secret --show-labels -n ${testName}`)
      .its('stdout')
      .should('match', /test-secret-equity.*level=staging,test=true/)
      .and('match', /test-secret-mix.*level=qa/)
      .and('match', /test-secret-set.*level=production,test=true/)
      .and('match', /all=true/)
    cy.visit(`/k8s/ns/${testName}/operators.coreos.com~v1alpha1~ClusterServiceVersion/mock-operator/test.tectonic.com~v1~MockResource/~new`);
    Operand.switchToFormView();
    cy.get('button#root_spec_k8sResourcePrefixNoFilter').click();
    cy.get('ul[role="listbox"]')
      .within(() => {
        cy.contains('test-secret-equity').as('equity').should('be.visible');
        cy.contains('test-secret-set').as('set').should('be.visible');
        cy.contains('test-secret-mix').as('mix').should('be.visible');
        cy.contains('test-secret-all').as('all').should('be.visible').click();
      })

    cy.get('button#root_spec_k8sResourcePrefixEquityFilter').click();
    cy.get('ul[role="listbox"]')
      .within(() => {
        cy.get('@set').should('not.exist');
        cy.get('@mix').should('not.exist');
        cy.get('@all').should('not.exist');
        cy.get('@equity').should('be.visible').click();
      })

    cy.get('button#root_spec_k8sResourcePrefixSetFilter').click();
    cy.get('ul[role="listbox"]')
      .within(() => {
        cy.get('@equity').should('be.visible');
        cy.get('@mix').should('not.exist');
        cy.get('@all').should('not.exist');
        cy.get('@set').should('be.visible').click();
      })

    cy.get('button#root_spec_k8sResourcePrefixMixedFilter').click();
    cy.get('ul[role="listbox"]')
      .within(() => {
        cy.get('@equity').should('not.exist');
        cy.get('@set').should('not.exist');
        cy.get('@all').should('not.exist');
        cy.get('@mix').should('be.visible').click();
      })

    cy.contains('No k8sResourcePrefixNone found').should('exist');
    Operand.submitCreation();

    cy.adminCLI(`oc get mockresource.test.tectonic.com example -n ${testName} -o yaml`)
      .its('stdout')
      .should('match', /k8sResourcePrefixEquityFilter: test-secret-equity/)
      .and('match', /k8sResourcePrefixMixedFilter: test-secret-mix/)
      .and('match', /k8sResourcePrefixNoFilter: test-secret-all/)
      .and('match', /k8sResourcePrefixSetFilter: test-secret-set/)
  });
})
