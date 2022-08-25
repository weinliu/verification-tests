type PDBParams = {
  name: string,
  label: string,
  value: string
}

export const pdbListPage = {
  createPDB: (params: PDBParams) => {
    const {name, label, value} = params;
    cy.get('[id="form"]').click();
    cy.get('[id="pdb-name"]').clear().type(name);
    cy.get('[data-test="tags-input"]').clear().type(label);
    cy.byButtonText('Requirement').parent().click();
    cy.get('.pf-c-dropdown__menu-item').contains('maxUnavailable').click();
    cy.get('[aria-label="minAvailable"]').clear().type(value);
    cy.get('[id="save-changes"]').click();
  },
}