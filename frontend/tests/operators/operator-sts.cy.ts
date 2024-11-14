import { Pages } from "views/pages";
import { operatorHubPage } from "../../views/operator-hub-page";

describe('Operators related features on sts cluster mode', () => {
  const params ={
    'ns': 'pro-ocp-sts',
    'csName': 'custom-catalogsource',
    'catalogsource': "Custom-Auto-Source",
    'operatorName': 'apicast',
    'subscriptionName': "apicast-operator"
  }

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc new-project ${params.ns}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.adminCLI(`oc create -f ./fixtures/operators/custom-catalog-source.json`);
    cy.checkCommandResult(`oc get catalogsource custom-catalogsource -n openshift-marketplace -o jsonpath='{.status.connectionState.lastObservedState}'`, 'READY');
    Pages.gotoCatalogSourcePage();
  });

  after(() => {
    cy.adminCLI(`oc delete project ${params.ns}`);
    cy.adminCLI(`oc delete catalogsource ${params.csName} -n openshift-marketplace`, { failOnNonZeroExit: false });
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`, { failOnNonZeroExit: false });
  });

  it('(OCP-66651,yanpzhan,UserInterface) Add support for Azure Workload Identity / Federated identity operator installs',{tags:['@userinterface','@e2e','admin']}, function () {
    let credentialMOde, infraPlatform, authIssuer;
    cy.exec(`oc get cloudcredential cluster --template={{.spec.credentialsMode}} --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result) => {
      credentialMOde = result.stdout;
      cy.exec(`oc get infrastructure cluster --template={{.status.platform}} --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result2) => {
        infraPlatform = result2.stdout;
        cy.exec(`oc get authentication cluster --template={{.spec.serviceAccountIssuer}} --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result3) => {
        authIssuer = result3.stdout;
        cy.log(`platform: ${infraPlatform} #########`);
        cy.log(`credentialMOde: ${credentialMOde} #########`);
        cy.log(`authIssuer: ${authIssuer} #########`);
        cy.isAzureWIFICluster(credentialMOde, infraPlatform, authIssuer).then(value => {
          if(value == false){
            cy.log('not Azure WIFI Cluster!!');
            this.skip();
          }
        });
        });
      });
    });
    operatorHubPage.checkSTSWarningOnOperator(`${params.operatorName}`, `${params.catalogsource}`, 'Workload Identity / Federated Identity Mode', `${params.ns}`, 'azure');
    cy.exec(`oc get subscriptions ${params.subscriptionName} -n ${params.ns} -o jsonpath='{.spec.config.env}' --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((output) => {
      expect(output.stdout).contains('testazureclientid');
      expect(output.stdout).contains('testazuretenantid');
      expect(output.stdout).contains('testazuresubscriptionid');
    });

  });

  it('(OCP-64758,yanpzhan,UserInterface) Warning user on operator item detail page if cluster is in sts model',{tags:['@userinterface','@e2e','admin','@rosa']}, function () {
    let credentialMOde, infraPlatform, authIssuer;
    cy.exec(`oc get cloudcredential cluster --template={{.spec.credentialsMode}} --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result) => {
      credentialMOde = result.stdout;
      cy.exec(`oc get infrastructure cluster --template={{.status.platform}} --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result2) => {
        infraPlatform = result2.stdout;
        cy.exec(`oc get authentication cluster --template={{.spec.serviceAccountIssuer}} --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result3) => {
        authIssuer = result3.stdout;
        cy.log(`platform: ${infraPlatform} #########`);
        cy.log(`credentialMOde: ${credentialMOde} #########`);
        cy.log(`authIssuer: ${authIssuer} #########`);
        cy.isAWSSTSCluster(credentialMOde, infraPlatform, authIssuer).then(value => {
          if(value == false){
   	    cy.log('not sts!!');
            this.skip();
          }
        });
        });
      });
    });

    operatorHubPage.checkSTSWarningOnOperator(`${params.operatorName}`, `${params.catalogsource}`, 'Cluster in STS Mode', `${params.ns}`, 'aws')
    cy.exec(`oc get subscriptions ${params.subscriptionName} -n ${params.ns} -o jsonpath='{.spec.config.env}' --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((output) => {
      expect(output.stdout).contains('testrolearn');
    });
  });

  it('(OCP-75413,xiyuzhao,UserInterface) Add support for GCP Workload Identity / Federated identity operator installs',{tags:['@userinterface','@e2e','admin']}, function () {
    cy.checkClusterType('isGCPWIFICluster').then(value => {
      if (value === false) {
        Cypress.log({message:'Skip case OCP-75413, cluster is not GCP WIFI enabled!!'})
        this.skip();
      }
    });
    const warnmessage = 'GCP Workload Identity / Federated Identity Mode'
    operatorHubPage.checkSTSWarningOnOperator(`${params.operatorName}`, `${params.catalogsource}`, `${warnmessage}`, `${params.ns}`, 'gcp')
    cy.adminCLI(`oc get subscriptions ${params.subscriptionName} -n ${params.ns} -o jsonpath='{.spec.config.env}'`, { timeout:120000}).then((output) => {
      expect(output.stdout).contains('testgcpprojectid');
      expect(output.stdout).contains('testgcppoolid');
      expect(output.stdout).contains('testgcpproviderid');
      expect(output.stdout).contains('testgcpemail');
    });
  });

  it('(OCP-71516,xiyuzhao,UserInterface) Add TLSProfiles and tokenAuthGCP annotation to Infrastructures features filter section',{tags:['@userinterface','@e2e','admin']}, function () {
    cy.checkClusterType('isGCPWIFICluster').then(value => {
      if (value === false) {
        cy.log('Skip case OCP-71516, cluster is not GCP WIFI enabled!!');
        this.skip();
      }
    })
    // Check the new annotation is listed on the Infrastructure filter list
    Pages.gotoOperatorHubPage();
    operatorHubPage.checkInfraFeaturesCheckbox("configurable-tls-ciphers");
    operatorHubPage.checkInfraFeaturesCheckbox("auth-token-gcp");
    // Check the annotation is added for the Operator
    operatorHubPage.filter(params.operatorName);
    operatorHubPage.clickOperatorTile(params.operatorName);
    cy.contains('h5', 'Infrastructure features')
      .parent()
      .within(() => {
        cy.contains('div', 'Auth Token GCP').should('exist');
        cy.contains('div', 'Configurable TLS ciphers').should('exist');
      });
  });
})
