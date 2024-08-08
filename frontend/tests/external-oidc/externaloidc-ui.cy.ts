import { commandLineToolsPage } from '../../views/command-line-tools-page';
import { nav } from 'upstream/views/nav';

describe('external oidc test - ui.', () => {
  it('(OCP-72252,yanpzhan,UserInterface) Console should work well on HCP cluster with Microsoft Entra ID as external OIDC',{tags:['@userinterface','@external-oidc-ui']}, () => {
    cy.uiLoginAzureExternalOIDC();
    nav.sidenav.clickNavLink(['User Management']);
    cy.get('a[data-test="nav"]').contains('Users').should('not.exist');
    cy.get('a[data-test="nav"]').contains('Groups').should('not.exist');
    commandLineToolsPage.goTo();
    cy.contains('button', 'Copy login command').click();
    commandLineToolsPage.checkExternalOIDCCopyLoginCommand();
    cy.byTestID('user-dropdown').click();
    cy.byTestID('copy-login-command').click();
    commandLineToolsPage.checkExternalOIDCCopyLoginCommand();
    cy.uiLogoutAzureExternalOIDC();
  });
})
