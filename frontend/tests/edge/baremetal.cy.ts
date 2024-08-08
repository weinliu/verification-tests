import { BareMetalHostPage } from "../../views/baremetalhost"
import { actionList } from "../../views/utils";
import { guidedTour } from '../../upstream/views/guided-tour';

describe('BareMetalHosts related features', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-63080,yanpzhan,UserInterface) Check functions in actions list for BMH',{tags:['@userinterface','e2e','admin']}, function () {
    cy.isEdgeCluster().then(value => {
      if(value == false){
        this.skip();
      }
    });
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.switchPerspective('Administrator');
    BareMetalHostPage.goToBMHList();
    BareMetalHostPage.createBMH("name63080", "34:48:ed:f3:88:c4", "redfish-virtualmedia://10.46.61.167:443/redfish/v1/Systems/System.Embedded.1", "root", "password");
    BareMetalHostPage.powerOffHost();
    actionList.clickActionItem('Power On');
    BareMetalHostPage.restartHost();
    BareMetalHostPage.editHost("test", "testpassword");
    cy.exec(`oc get secret name63080-bmc-secret -n openshift-machine-api -o json --kubeconfig=${Cypress.env('KUBECONFIG_PATH')} | jq -r '.data.username' | base64 -d`).then((output) => {
      expect(output.stdout).contain('test');
    });
    cy.exec(`oc get secret name63080-bmc-secret -n openshift-machine-api -o json --kubeconfig=${Cypress.env('KUBECONFIG_PATH')} | jq -r '.data.password' | base64 -d`).then((output) => {
      expect(output.stdout).contain('testpassword');
    });
    BareMetalHostPage.editHostNegativeTest("30:48:ed:f3:88:e4", "redfish-virtualmedia://10.46.62.107:443/redfish/v1/Systems/System.Embedded.1");

    BareMetalHostPage.deleteHost();
    cy.adminCLI(`oc get baremetalhosts.metal3.io --all-namespaces`).then((output) => {
      expect(output.stdout).not.contain('name63080');
    });
  });
})
