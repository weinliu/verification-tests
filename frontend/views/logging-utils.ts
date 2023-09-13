import { listPage } from "../upstream/views/list-page";

//If specific channel/catsrc needed for testing, export the values using CYPRESS_EXTRA_PARAM before running the logging tests
//ex: export CYPRESS_EXTRA_PARAM='{"openshift-logging": {"cluster-logging": {"channel": "stable-5.8", "version" : "5.8.0", "source": "qe-app-registry"}, "elasticsearch-operator": {"channel": "stable-5.8", "version" : "5.8.0", "source": "qe-app-registry"}, "loki-operator": {"channel": "stable-5.8", "version" : "5.8.0", "source": "qe-app-registry"}}}'
const extraParam = JSON.stringify(Cypress.env("EXTRA_PARAM"))
const loggingParam = (extraParam != undefined) ? JSON.parse(extraParam) : null;

export const catalogSource = {
  //set channel
  channel: (packageName) => {
    let channel = (loggingParam != null) ? loggingParam['openshift-logging'][`${packageName}`]['channel'] : null;
    if(channel == null){
      channel = "stable";
    }
    return channel;
  },
  //set version (availabe for OCP >= 4.14)
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
  installOperator: (namespace, packageName, csName, channelName?, version?, enablePlugin?: boolean) => {
    cy.exec(`oc get csv -n ${namespace} -l operators.coreos.com/${packageName}.${namespace} -ojsonpath={.items[].status.phase} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
      if (result.stdout.includes("Succeeded")) {
        cy.log(`operator ${packageName} is installed in ${namespace} project`)
      } else {
        cy.visit(`/operatorhub/subscribe?pkg=${packageName}&catalog=${csName}&catalogNamespace=openshift-marketplace&targetNamespace=undefined`);
        if (channelName){
          if (Cypress.$(`[data-test="${channelName}-radio-input"]`).length > 0 ){
            cy.get(`[data-test="${channelName}-radio-input"]`).click();
          } else {
            if (Cypress.$('#pf-select-toggle-id-16').length > 0) {
              cy.get('#pf-select-toggle-id-16').click(); 
              cy.get(`#${channelName}`).should('exist').click();
            }
          }
        }
        if (version){
          if (Cypress.$('.co-operator-version__select').length > 0) {
            cy.get('#pf-select-toggle-id-57').click();
            if (Cypress.$(`#${version}`).length > 0 ) {
              cy.get(`#${version}`).click();
            } else {
              cy.get('.pf-c-select__menu-item').first().click();
            }
          }
        }
        cy.exec(`oc get ns ${namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
          if(result.stderr.includes('NotFound')){
            cy.get('input[data-test="enable-monitoring"]').click();
          } else {
            cy.contains('Namespace already exists').should('be.visible')
          }
        })
        if(enablePlugin){
          cy.get('input[name="logging-view-plugin"][data-test="Enable-radio-input"]').click();
        }
        cy.get('[data-test="install-operator"]').click();
        cy.contains('View Operator').should('be.visible');
      }
    })
  },
  uninstallOperator: (operatorName, nameSpace, packageName) => {
    cy.exec(`oc get sub ${packageName} -n ${nameSpace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false}).then(result => {
      if(!result.stderr.includes('NotFound')){
        cy.visit(`/k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion`)
        cy.byLegacyTestID(`resource-title`).should('be.visible')
        listPage.rows.clickKebabAction(`${operatorName}`,"Uninstall Operator");
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
    cy.exec(`oc wait --timeout=240s --for=condition=ready pod -l ${label} -n ${namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {timeout: 240000, failOnNonZeroExit: false })
  },
  createClusterLoggingViaYamlView: (namespace: string, file: string) => {
    logUtils.removeClusterLogging(namespace)
    cy.visit(`/k8s/ns/${namespace}/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
    cy.get('[title="clusterloggings.logging.openshift.io"]').click();
    cy.contains('Create ClusterLogging').should('be.visible');
    cy.get('[data-test="item-create"]').click();
    cy.get('[data-test="yaml-view-input"]').click();
    cy.get('.ocs-yaml-editor__root')
        .selectFile(`./fixtures/logging/${file}`, {action: 'drag-drop'})
    cy.wait(10)
    cy.get('[data-test="save-changes"]').click()
  },
  removeClusterLogging:(namespace: string) => {
    cy.exec(`oc delete cl instance -n ${namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
    cy.exec(`oc delete pvc -n ${namespace} -l logging-cluster=elasticsearch --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, {failOnNonZeroExit: false})
  }
};
