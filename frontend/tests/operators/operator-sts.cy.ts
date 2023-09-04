import { operatorHubPage } from "../../views/operator-hub-page";

describe('Operators related features on sts cluster mode', () => {
  before(() => {
    cy.adminCLI(`oc new-project pro-ocp-64758`);
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc create -f ./fixtures/test-cs.yaml`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc delete project pro-ocp-64758`);
    cy.exec(`oc delete catalogsources.operators.coreos.com uitestcs -n openshift-marketplace --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false });
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false });
  });

  it('(OCP-64758,yanpzhan) Warning user on operator item detail page if cluster is in sts model', {tags: ['e2e','admin','@osd-ccs','@rosa']}, function () {
    let credentialMOde, infraPlatform, authIssuer;
    cy.exec(`oc get cloudcredential cluster --template={{.spec.credentialsMode}} --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result) => {
      credentialMOde = result.stdout;
      cy.exec(`oc get infrastructure cluster --template={{.status.platform}} --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result2) => {
        infraPlatform = result2.stdout;
        cy.exec(`oc get authentication cluster --template={{.spec.serviceAccountIssuer}} --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result3) => {
        authIssuer = result3.stdout;
        cy.log(`platform: ${infraPlatform} #########`);
        cy.log(`credentialMOde: ${credentialMOde} #########`);
        cy.log(`authIssuer: ${authIssuer} #########`);
        cy.isAWSSTSCluster(credentialMOde, infraPlatform, authIssuer).then(value => {
          if(value == false){
   	    cy.log('not sts!!');
            this.skip();
          }
        });
        });
      });
    });

    operatorHubPage.checkSTSwarningOnOperator('apicast', 'UI-Test-CS', 'pro-ocp-64758', 'testrolearn')
    cy.exec(`oc get subscriptions apicast-operator -n pro-ocp-64758 -o jsonpath='{.spec.config.env}' --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((output) => {
      expect(output.stdout).contains('testrolearn');
    });
  });

})
