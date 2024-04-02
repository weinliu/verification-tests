export const storage = {
  createPVC: (pvc_name: string, size: string, unit: string) => {
    cy.get('#pvc-name').clear().type(pvc_name);
    cy.get('#request-size-input').type(size);
    cy.get('button[data-test-id="dropdown-button"]').click();
    cy.get('button[data-test-id="dropdown-menu"]').contains(`${unit}`).click();
    cy.get('#save-changes').click();
  },
  createVolumnSnapshot: (pvc_name) => {
    cy.get('button[data-test="pvc-dropdown"]').click();
    cy.get(`a[id="${pvc_name}-PersistentVolumeClaim-link"]`).click();
    cy.get('#save-changes').click();
  },
  checkSCDropdownText: (page: string) => {
    const button_selector = page === 'clone'
    ? 'button[data-test="storage-class-dropdown"]'
    : 'button[id="restore-storage-class"]';
    cy.get(button_selector).click();
    return cy.get('div[class*="text-muted"]');
  }
}