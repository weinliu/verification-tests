describe('console configs features', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-53787,yanpzhan,UserInterface) Backend changes to add nodeArchitectures value to console-config file',{tags:['@userinterface','e2e','admin','@osd-ccs','@rosa']}, () => {
    let $architectureType;
    cy.exec(`oc get nodes -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | awk '/architecture:/ {print $2}' | sort | uniq`, { failOnNonZeroExit: false }).then((result) => {
      $architectureType = result.stdout;
    });

    cy.exec(`oc get configmaps console-config -n openshift-console -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | awk '$1 == "-"{ if (key == "nodeArchitectures:") print $NF; next } {key=$1}'`, { failOnNonZeroExit: false }).then((output) => {
      expect(output.stdout).to.include($architectureType);
    });
  });
})

