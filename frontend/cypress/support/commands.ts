import { nav } from '../../upstream/views/nav';

declare global {
    namespace Cypress {
        interface Chainable<Subject> {
            switchPerspective(perspective: string);
	          uiLogin(provider: string, username: string, password: string);
            uiLogout();
            cliLogin();
            cliLogout();
            adminCLI(command: string);
            retryTask(condition, expectedoutput, options);
            checkCommandResult(condition, expectedoutput, options);
            hasWindowsNode();
            isEdgeCluster();
            isAWSSTSCluster();
            isPlatformSuitableForNMState();
	    isManagedCluster();
        }
    }
}

const kubeconfig = Cypress.env('KUBECONFIG_PATH');
const DEFAULT_MAX_RETRIES = 3;
const DEFAULT_RETRY_INTERVAL = 10000; // milliseconds

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

// to avoid influence from upstream login change
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
    }else if ($body.find('li.idp').length > 0) {
      //using the last idp if doesn't provider idp name
      cy.get('li.idp').last().click();
    }
  });
  cy.get('#inputUsername').type(username);
  cy.get('#inputPassword').type(password);
  cy.get('button[type=submit]').click();
  cy.byTestID("username")
    .should('be.visible');
  })
  cy.visit('/');
});

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
  cy.exec(`${command} --kubeconfig ${kubeconfig}`, { failOnNonZeroExit: false })
});

Cypress.Commands.add('retryTask', (command, expectedOutput, options) => {
  const { retries = DEFAULT_MAX_RETRIES, interval = DEFAULT_RETRY_INTERVAL } = options || {};

  const retryTaskFn = (currentRetries) => {
    return cy.adminCLI(command)
      .then(result => {
        if (result.stdout.includes(expectedOutput)) {
          return cy.wrap(true);
        } else if (currentRetries < retries) {
          return cy.wait(interval).then(() => retryTaskFn(currentRetries + 1));
        } else {
          return cy.wrap(false);
        }
      });
  };
  return retryTaskFn(0);
});

Cypress.Commands.add("checkCommandResult", (command, expectedoutput, options) => {
  return cy.retryTask(command, expectedoutput, options)
    .then(conditionMet =>{
      if (conditionMet) {
        return;
      } else {
        throw new Error(`"${command}" failed to meet expectedoutput ${expectedoutput} within ${options.retries} retries`);
      }
    })
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
Cypress.Commands.add("isAWSSTSCluster", (credentialMode: string, infraPlatform: string, authIssuer: string) => {
  if(credentialMode == 'Manual' && infraPlatform == 'AWS' && authIssuer != ''){
    cy.log('Testing on AWS STS cluster!');
    return cy.wrap(true);
  }else{
    cy.log('Not AWS STS cluster, skip!');
    return cy.wrap(false);
  }
});
Cypress.Commands.add("isAzureWIFICluster", (credentialMode: string, infraPlatform: string, authIssuer: string) => {
  if(credentialMode == 'Manual' && infraPlatform == 'Azure' && authIssuer != ''){
    cy.log('Testing on Azure WIFI cluster!');
    return cy.wrap(true);
  }else{
    cy.log('Not Azure WIFI cluster, skip!');
    return cy.wrap(false);
  }
});

Cypress.Commands.add("isPlatformSuitableForNMState", () => {
  cy.exec(`oc get infrastructure cluster -o jsonpath={.spec.platformSpec.type} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result) => {
    if( result.stdout == 'BareMetal' || result.stdout == 'None' || result.stdout == 'VSphere' || result.stdout == 'OpenStack'){
      cy.log("Testing on baremetal/vsphere/openstack.");
      return cy.wrap(true);
    } else {
      cy.log("Skipping for unsupported platform, not baremetal/vsphere/openstack!");
      return cy.wrap(false);
    }
  });
});

Cypress.Commands.add("isManagedCluster", () => {
  let brand;
  cy.window().then((win: any) => {
    brand = win.SERVER_FLAGS.branding;
    cy.log(`${brand}`);
    if(brand == 'rosa' || brand == 'dedicated'){
      cy.log('Testing on Rosa/OSD cluster!');
      return cy.wrap(true);
    } else {
      cy.log('Not Rosa/OSD cluster. Skip!');
      return cy.wrap(false);
    }
  });
});

Cypress.Commands.add("isIPICluster", () => {
  cy.exec(`oc get machines.machine.openshift.io -n openshift-machine-api --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result) => {
    if( result.stdout.includes('Running') ){
      cy.log("Testing on IPI cluster!");
      return cy.wrap(true);
    } else {
      cy.log("Not IPI cluster. Skip!");
      return cy.wrap(false);
    }
  });
});
