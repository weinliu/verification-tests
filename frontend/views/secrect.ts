export const Secrets = {
    gotoSecretsPage: (ns:string) => {
        cy.visit(`/k8s/ns/${ns}/secrets`);
        cy.get('table').should('exist');
    },
    revealValue: () => cy.contains('Reveal value').click(),
    addKeyValue:(key: string, value: string) => {
        cy.contains('button', 'Add key/value').click();
        cy.get('input[data-test="secret-key"]').last().clear().type(key);
        cy.get('textarea[data-test-id="file-input-textarea"]').last().clear().type(value)
    },
    validKeyValueExist :(key: string, value: string) => {
        // Just for one new add key/value
        Secrets.revealValue();
        cy.get('[data-test="secret-data-term"]').first().should('have.text', key);
        cy.get('code').first().should('have.text', value);
    },
    createImagePullSecret: (secretname: string, address: string, username: string, password: string, email: string) => {
      cy.contains('Create').click();
      cy.contains('Image pull secret').click();
      cy.get('input[data-test="secret-name"').type(`${secretname}`);
      cy.get('input[data-test="image-secret-address"').type(`${address}`);
      cy.get('input[data-test="image-secret-username"').type(`${username}`);
      cy.get('input[data-test="image-secret-password"').type(`${password}`);
      cy.get('input[data-test="image-secret-email"').type(`${email}`);
      cy.byTestID('save-changes').click();
    }
}
