import { nav } from '../../upstream/views/nav';

declare global {
    namespace Cypress {
        interface Chainable<Subject> {
            switchPerspective(perspective: string);
            uiLogout();
            cliLogin();
            cliLogout();
            adminCLI(command: string);
            hasWindowsNode();
            isEdgeCluster();
            isAWSSTSCluster();
        }
    }
}

const kubeconfig = Cypress.env('KUBECONFIG_PATH')

Cypress.Commands.add("switchPerspective", (perspective: string) => {

    /* if side bar is collapsed then expand it
    before switching perspecting */
    cy.get('body').then((body) => {
        if (body.find('.pf-m-collapsed').length > 0) {
            cy.get('#nav-toggle').click()
        }
    });
    nav.sidenav.switcher.changePerspectiveTo(perspective);
    nav.sidenav.switcher.shouldHaveText(perspective);
});

/* Save for debuggin upstream login change
Cypress.Commands.add('uiLogin', (provider: string, username: string, password: string)=> {
  cy.clearCookie('openshift-session-token');
  cy.visit('/');
  cy.window().then((win: any) => {
    if(win.SERVER_FLAGS?.authDisabled) {
      cy.task('log', 'Skipping login, console is running with auth disabled');
      return;
    }
  cy.get('[data-test-id="login"]').should('be.visible');
  cy.get('body').then(($body) => {
    if ($body.text().includes(provider)) {
      cy.contains(provider).should('be.visible').click();
    }
  });
  cy.get('#inputUsername').type(username);
  cy.get('#inputPassword').type(password);
  cy.get('button[type=submit]').click();
  cy.byTestID("username")
    .should('be.visible');
  })
  cy.visit('/');
}); */

Cypress.Commands.add('uiLogout', () => {
  cy.window().then((win: any) => {
    if (win.SERVER_FLAGS?.authDisabled){
      cy.log('Skipping logout, console is running with auth disabled');
      return;
    }
    cy.log('Loggin out UI');
    cy.byTestID('user-dropdown').click();
    cy.byTestID('log-out').should('be.visible');
    cy.byTestID('log-out').click({ force: true });
  })
});

Cypress.Commands.add("cliLogin", () => {
  cy.exec(`oc login -u ${Cypress.env('LOGIN_USERNAME')} -p ${Cypress.env('LOGIN_PASSWORD')} ${Cypress.env('HOST_API')} --insecure-skip-tls-verify=true`, { failOnNonZeroExit: false }).then(result => {
    cy.log(result.stderr);
    cy.log(result.stdout);
  });
});

Cypress.Commands.add("cliLogout", () => {
  cy.exec(`oc logout`, { failOnNonZeroExit: false }).then(result => {
    cy.log(result.stderr);
    cy.log(result.stdout);
  });
});

Cypress.Commands.add("adminCLI", (command: string) => {
  cy.log(`Run admin command: ${command}`)
  cy.exec(`${command} --kubeconfig ${kubeconfig}`)
});

const hasWindowsNode = () :boolean => {
  cy.exec(`oc get node -l kubernetes.io/os=windows --kubeconfig ${kubeconfig}`).then((result) => {
      if(!result.stdout){
        cy.log("Testing on cluster without windows node. Skip this windows scenario!");
        return false;
      } else {
        cy.log("Testing on cluster with windows node.");
        return cy.wrap(true);
      }
  });
};
Cypress.Commands.add("hasWindowsNode", () => {
  return hasWindowsNode();
});
Cypress.Commands.add("isEdgeCluster", () => {
  cy.exec(`oc get infrastructure cluster -o jsonpath={.spec.platformSpec.type} --kubeconfig ${kubeconfig}`, { failOnNonZeroExit: false }).then((result) => {
      cy.log(result.stdout);
      if ( result.stdout == 'BareMetal' ){
         cy.log("Testing on Edge cluster.");
         return cy.wrap(true);
      }else {
         cy.log("It's not Edge cluster. Skip!");
         return cy.wrap(false);
      }
    });
});
Cypress.Commands.add("isAWSSTSCluster", (credentialMOde: string, infraPlatform: string, authIssuer: string) => {
  if(credentialMOde == 'Manual' && infraPlatform == 'AWS' && authIssuer != ''){
    cy.log('Testing on STS cluster!');
    return cy.wrap(true);
  }else{
    cy.log('Not STS cluster, skip!');
    return cy.wrap(false);
  }
});
