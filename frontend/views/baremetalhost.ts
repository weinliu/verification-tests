import { actionList } from "views/utils"
export const BareMetalHostPage = {
  goToBMHList: () => cy.visit(`/k8s/ns/openshift-machine-api/metal3.io~v1alpha1~BareMetalHost`).get('[data-test-id="resource-title"]').should('be.visible'),
  createBMH: (bmhName, macAddress, bmcAddress, userName, password) => {
    cy.byLegacyTestID('dropdown-button').click();
    cy.get('#dialog-link').click();
    cy.get('input[name="name"]').type(bmhName);
    cy.get('input[name="bootMACAddress"]').type(macAddress);
    cy.get('input[name="BMCAddress"]').type(bmcAddress);
    cy.get('input[name="username"]').type(userName);
    cy.get('input[name="password"]').type(password);
    actionList.submitAction();
  },
  powerOffHost: () => {
    actionList.clickActionItem('Power Off');
    cy.get('#host-force-off').click();
    actionList.submitAction();
  },
  restartHost: () => {
    actionList.clickActionItem('Restart');
    actionList.submitAction();
  },
  editHostNegativeTest: (macAddress, bmcAddress) => {
    actionList.clickActionItem('Edit Bare Metal Host');
    cy.get('input[name="bootMACAddress"]').clear().type(macAddress);
    cy.get('input[name="BMCAddress"]').clear().type(bmcAddress);
    actionList.submitAction();
    cy.contains('BMC address can not be changed').should('exist');
    cy.contains('bootMACAddress can not be changed').should('exist');
    cy.byLegacyTestID('cancel-button').click();
  },

  editHost: (userName, password) => {
    actionList.clickActionItem('Edit Bare Metal Host');
    cy.get('input[name="username"]').clear().type(userName);
    cy.get('input[name="password"]').clear().type(password);
    actionList.submitAction();
  },
  deleteHost: () => {
    actionList.clickActionItem('Delete Bare Metal Host');
    actionList.submitAction();
  }

}
