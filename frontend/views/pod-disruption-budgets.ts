type PDBParams = {
  name: string,
  value: string
}

export const pdbListPage = {
  createPDB: (params: PDBParams) => {
    const {name, value} = params;
    cy.get('[id="form"]').click();
    cy.get('[id="pdb-name"]').clear().type(name);
    cy.byButtonText('Requirement').parent().click();
    cy.get('span').contains('maxUnavailable').parent().parent('button').click();
    cy.get('[name="availability requirement value"]').clear().type(value);
    cy.get('[id="save-changes"]').click();
  },
}