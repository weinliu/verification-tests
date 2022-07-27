export const Secrets = {
    gotoSecretsPage: (ns:string) => {
        cy.visit(`/k8s/ns/${ns}/secrets`);
        cy.get('table').should('exist');
    },
    addKeyValue:(key: string, value: string) => {
        cy.contains('button', 'Add key/value').click();
        cy.get('input[data-test="secret-key"]').last().clear().type(key);
        cy.get('textarea[data-test-id="file-input-textarea"]').last().clear().type(value)
    },
    validKeyValueExist :(key: string, value: string) => {
        // Just for one new add key/value
        cy.contains('button', 'Reveal values').click();
        cy.get('.secret-data dt').first().should('have.text', key);
        cy.get('code').first().should('have.text', value);
    }
}