import { catalogSources } from "views/catalog-source";
import { catalogSource, logUtils } from "../../views/logging-utils";
import { LokiUtils } from "views/logging-loki-utils";

describe('Loki log based alerts on dev-console', () => {
  const appNs = "test-ocp65686";
  const devConsolePathForAlert = 'dev-monitoring/ns/' + appNs + '/alerts';
  const monitoringAlertsPath = 'monitoring/alerts'
  let ocpVersion: string

  const CLO = {
    namespace: "openshift-logging",
    packageName: "cluster-logging",
    operatorName: "Red Hat OpenShift Logging"
  };
  const LO = {
    namespace: "openshift-operators-redhat",
    packageName: "loki-operator",
    operatorName: "Loki Operator"
  };
  const LokiStack = {
    name: "logging-lokistack-ocp65686",
    bucketName: "logging-loki-bucket-ocp65686",
    secretName: "logging-loki-secret-ocp65686",
    tSize: "1x.demo"

  };

  before(() => {

    cy.adminCLI(`oc adm groups new cluster-admin`);
    cy.adminCLI(`oc adm groups add-users cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    cy.adminCLI(`oc adm policy add-cluster-role-to-group cluster-admin cluster-admin`)

    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));

    //Install logging operators if needed
    catalogSource.sourceName(CLO.packageName).then((csName) => {
      logUtils.installOperator(CLO.namespace, CLO.packageName, csName, catalogSource.channel(CLO.packageName), catalogSource.version(CLO.packageName), true, CLO.operatorName);
    });
    catalogSource.sourceName(LO.packageName).then((csName) => {
      logUtils.installOperator(LO.namespace, LO.packageName, csName, catalogSource.channel(LO.packageName), catalogSource.version(LO.packageName), false, LO.operatorName);
    });

    catalogSources.getOCPVersion()
    cy.get('@VERSION').then((VERSION) => {
      ocpVersion = `${VERSION}`
    })
  });

  after(() => {
    cy.exec(`oc delete project ${appNs} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false });
    logUtils.removeClusterLogging(CLO.namespace);
    logUtils.removeLokistack(LokiStack.name, CLO.namespace)
    cy.exec(`oc delete secret logging-loki-secret-ocp65686 -n ${CLO.namespace}`,{ failOnNonZeroExit: false })
    cy.exec(`oc adm policy remove-cluster-role-from-group cluster-admin cluster-admin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false });
    cy.exec(`oc delete group cluster-admin --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false });   
  });

  it('(OCP-65686,kbharti,Logging) Validate Loki log based alerts on Console', { tags: ['e2e', 'admin', '@smoke', '@logging'] }, function () {

    if (ocpVersion < '4.13') {
      // Skipping the test on OCP versions below 4.13 since Alerts are available on 4.13+
      this.skip();
    }

    cy.exec(`oc new-project ${appNs} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false });
    cy.exec(`oc label namespace ${appNs} openshift.io/cluster-monitoring=true`, { failOnNonZeroExit: false });
    cy.exec(`oc new-app -f ./fixtures/logging/container_json_log_template.json -n ${appNs} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false });

    //Create bucket and secret for lokistack
    LokiUtils.prepareResourcesForLokiStack(CLO.namespace, LokiStack.secretName, LokiStack.bucketName);

    // Deploy lokistack under openshift-logging
    LokiUtils.getStorageClass().then((SC) => {
      LokiUtils.getPlatform()
      cy.get<string>('@ST').then(ST => {
        cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/lokistack-sample.yaml -n ${CLO.namespace} -p NAME=${LokiStack.name} SIZE=${LokiStack.tSize} SECRET_NAME=${LokiStack.secretName} STORAGE_TYPE=${ST} STORAGE_CLASS=${SC} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, { failOnNonZeroExit: false })
          .then(output => {
            expect(output.stderr).not.contain('Error');
          })
      })
    })
    logUtils.waitforPodReady(CLO.namespace, 'app.kubernetes.io/name=lokistack');

    //Create CLF
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/clf-forward-default.yaml -n ${CLO.namespace} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, { failOnNonZeroExit: false })
      .then(output => {
        expect(output.stderr).not.contain('Error');
      })
    //Create CL instance
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/cl_logStore_lokistack.yaml -n ${CLO.namespace} -p LOKISTACKNAME=${LokiStack.name} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, { failOnNonZeroExit: false })
      .then(output => {
        expect(output.stderr).not.contain('Error');
      })
    logUtils.waitforPodReady(CLO.namespace, 'component=collector');
    logUtils.waitforPodReady(CLO.namespace, 'app.kubernetes.io/name=logging-view-plugin');

    // Create application alert and wait for ruler to restart
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/loki-app-alert.yaml -n ${appNs} -p NAMESPACE=${appNs} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`)
      .then(output => {
        expect(output.stderr).not.contain('Error');
      })
    logUtils.waitforPodReady(CLO.namespace, 'app.kubernetes.io/component=ruler');

    // Provide RBAC to user for accessing application logs on appNs namespace
    cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} process -f ./fixtures/logging/app-logs-rbac.yaml -p NAMESPACE=${appNs} -p USERNAME=${Cypress.env('LOGIN_USERNAME')} | oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} apply -f -`, { failOnNonZeroExit: false })
      .then(output => {
        expect(output.stderr).not.contain('Error');
      })

    // Validate Alert on Admin console 
    cy.visit(monitoringAlertsPath);
    cy.get('button[aria-label="Options menu"]').should('be.visible');
    cy.get('button[aria-label="Options menu"]').click();
    cy.get('[id="user"]').should('be.visible').click();
    cy.get('[id="pending"]').should('be.visible').click();
    cy.contains('a', 'MyAppLogVolumeIsHigh').should('be.visible');

    cy.adminCLI(`oc adm policy remove-cluster-role-from-group cluster-admin cluster-admin`);
    cy.adminCLI(`oc delete group cluster-admin`);

    cy.uiLogout();

    // Dev Console test
    if (ocpVersion > '4.13') {
      // Provide roles to the user to access Alerts on dev-console
      cy.adminCLI(`oc adm policy add-role-to-user admin ${Cypress.env('LOGIN_USERNAME')} -n ${appNs}`);
      cy.adminCLI(`oc adm policy add-role-to-user cluster-monitoring-view ${Cypress.env('LOGIN_USERNAME')} -n ${appNs}`);
      cy.adminCLI(`oc adm policy add-role-to-user monitoring-rules-edit ${Cypress.env('LOGIN_USERNAME')} -n ${appNs}`);

      // Login with regular user and check for Alert on dev-console
      cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
      cy.visit(devConsolePathForAlert);

      // Wait for the Alert to be visible on UI
      cy.get('table', { timeout: 60000 }).contains('td', 'MyAppLogVolumeIsHigh').should('be.visible');
    }
  });
});
