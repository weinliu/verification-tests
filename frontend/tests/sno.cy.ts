describe('console feature on sno cluster', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-39677,yanpzhan,UserInterface) Console supports on single-node cluster',{tags:['@userinterface','e2e','admin']}, () => {
    cy.exec(`oc get infrastructures.config.openshift.io cluster --template={{.status.controlPlaneTopology}} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result) => {
      if(result.stdout == 'SingleReplica') {
        cy.log("Testing on SNO cluster.");
        cy.exec(`oc get deployments -n openshift-console -o jsonpath='{.items[*].spec.replicas}' --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((output) => {
          expect(output.stdout).to.equal('1 1');
        });
        cy.exec(`oc get deployments -n openshift-console -o jsonpath='{.items[*].spec.template.spec.affinity}' --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((output) => {
          expect(output.stdout).to.equal('{} {}');
        });
      } else {
        cy.log("Testing on none-SNO cluster.");
      }
    });
  });

})

