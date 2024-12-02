import { listPage } from "../upstream/views/list-page";
import { operatorHubPage } from "./operator-hub-page";

//If specific channel/catsrc needed for testing, export the values using CYPRESS_EXTRA_PARAM before running the logging tests
//ex: export CYPRESS_EXTRA_PARAM='{"openshift-logging": {"cluster-logging": {"channel": "stable-5.z", "version" : "5.z.z", "source": "qe-app-registry"}, "loki-operator": {"channel": "stable-5.z", "version" : "5.z.z", "source": "qe-app-registry"}}}'
const extraParam = JSON.stringify(Cypress.env("EXTRA_PARAM"))
const loggingParam = (extraParam != undefined) ? JSON.parse(extraParam) : null;

export const catalogSource = {
  //set channel
  channel: (packageName) => {
    let channel = (loggingParam != null) ? loggingParam['openshift-logging'][`${packageName}`]['channel'] : null;
    if(channel == null){
      channel = "stable-6.1";
    }
    return channel;
  },
  version: (packageName) => {
    let version = (loggingParam != null) ? loggingParam['openshift-logging'][`${packageName}`]['version'] : null;
    return version;
  },
  //set source namespace
  nameSpace: (packageName) => {
    let namespace = (loggingParam != null) ? loggingParam['openshift-logging'][`${packageName}`]['catsrc-namespace'] : null;
    if(namespace == null) {
      namespace = "openshift-marketplace";
    }
    return namespace;
  },
  //set source and check if the packagemanifest exists or not
  sourceName: (packageName) => {
    let csName = (loggingParam != null) ? loggingParam['openshift-logging'][`${packageName}`]['source'] : null;
    if(csName == null) {
      return catalogSource.qeCatSrc();
    } else {
      return cy.exec(`oc get catsrc ${csName} -n ${catalogSource.nameSpace(packageName)} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
      .then(result => {
        if(!result.stderr.includes('NotFound')) {
          return csName;
        } else {
          return "redhat-operators";
        }
      })
    }
  },
  qeCatSrc: () => {
    return cy.exec(`oc get catsrc -n openshift-marketplace qe-app-registry --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
    .then(result => {
        if(!result.stderr.includes('NotFound')) {
          return "qe-app-registry";
        } else {
          return "redhat-operators";
        }
    })
  }
};

export const logUtils = {
  installOperator: (namespace, packageName, csName, channelName?, version?, operatorName?) => {
    cy.exec(`oc get csv -n ${namespace} -l operators.coreos.com/${packageName}.${namespace} -ojsonpath='{.items[].status.phase}' --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
      if (result.stdout.includes("Succeeded")) {
        cy.log(`operator ${packageName} is installed in ${namespace} project`)
      } else {
        cy.visit(`/operatorhub/subscribe?pkg=${packageName}&catalog=${csName}&catalogNamespace=openshift-marketplace&targetNamespace=undefined`);
        if (channelName){
          cy.get('button[data-test="operator-channel-select-toggle"]').click();
          cy.get(`[id="${channelName}"`).click();
          if (version) {
            cy.get('button[data-test="operator-version-select-toggle"]').eq(1).click();
            cy.get(`[id="${version}"`).click();
          }
        }
        cy.exec(`oc get ns ${namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
          if(result.stderr.includes('NotFound')){
            cy.get('input[data-test="enable-monitoring"]').click();
          }
        })
        cy.get('[data-test="install-operator"]').click();
        cy.get('#operator-install-page').should('exist')
        if (operatorName) {
          cy.visit(`/k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
          cy.contains('Installed Operators').should('exist');
          operatorHubPage.checkOperatorStatus(`${operatorName}`, 'Succeeded');
        }
      }
    })
  },
  uninstallOperator: (operatorName, nameSpace, packageName) => {
    cy.exec(`oc get sub ${packageName} -n ${nameSpace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
      if(!result.stderr.includes('NotFound')){
        cy.visit(`/k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
        cy.byLegacyTestID(`resource-title`).should('be.visible');
        listPage.rows.clickKebabAction(`${operatorName}`,"Uninstall Operator");
        cy.get('[name="form"]').then($body => {
          if ($body.find('[data-test="Delete all operand instances for this operator__checkbox"]').length) {
            cy.get('[data-test="Delete all operand instances for this operator__checkbox"]').click();
          }
        });
        cy.get('#confirm-action').click();
        cy.get(`[data-test-operator-row="${operatorName}"]`).should('not.exist');
      }
    })
  },
  deleteResourceByName: (kind: string, res_name: string, namespace: string) => {
    cy.exec(`oc delete ${kind} ${res_name} -n ${namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
  },
  deleteResourceByLabel: (kind: string, namespace: string, label: string) => {
    cy.exec(`oc delete ${kind} -n ${namespace} -l ${label} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
  },
  deleteNamespace: (namespace: string) => {
    cy.exec(`oc delete ns ${namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
  },
  waitforPodReady: (namespace: string, label: string) => {
    cy.exec(`oc wait --timeout=240s --for=condition=ready pod -l ${label} -n ${namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {timeout: 240000})
  },
  removeLokistack: (lokiName: string, namespace: string) => {
    cy.exec(`oc delete lokistack ${lokiName} -n ${namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
    cy.exec(`oc delete pvc -l app.kubernetes.io/instance=${lokiName} -n ${namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
  }
};
