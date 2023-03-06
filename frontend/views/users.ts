export const createRoleBinding = {
  setRoleBindingName: (rb_name: string) => {
    cy.get('#role-binding-name')
      .clear()
      .type(rb_name);
  },
  selectRBNamespace: (namespace_name: string) => {
    cy.get('#ns-dropdown').click();
    cy.byLegacyTestID('dropdown-text-filter')
      .type(namespace_name);
    cy.get('.co-resource-item__resource-name')
      .contains(namespace_name)
      .click();
  },
  selectRoleName: (role_name: string) => {
    cy.get('#role-dropdown').click();
    cy.byLegacyTestID('dropdown-text-filter')
      .type(role_name);
    cy.get('.co-resource-item__resource-name')
      .contains(role_name)
      .click();   
  },
  selectSubjectKind: (subject_kind: string) => {
    cy.byTestID(`${subject_kind}-radio-input`)
      .click({force: true});
  },
  setSubjectName: (subject_name: string) => {
    cy.get('#subject-name')
      .clear()
      .type(subject_name);
  },
  clickSubmitButton: () => {
    cy.get('#save-changes').click();
  }
}