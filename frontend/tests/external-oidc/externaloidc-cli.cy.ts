describe('external oidc test - cli.', () => {
  it('(OCP-71561,xxia,Auth) HCP authentication should work well with Microsoft Entra ID as external OIDC - cli', {tags: ['@external-oidc']}, () => {
    cy.cliLoginAzureExternalOIDC();
  });
})
