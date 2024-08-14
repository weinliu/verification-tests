import { Pages } from "views/pages";
import { operatorHubPage, operatorHubModal, OperatorHubSelector, Operand, installedOperators } from "../../views/operator-hub-page";
import { listPage } from "upstream/views/list-page";
import { detailsPage } from "upstream/views/details-page";

describe('Operator Hub tests', () => {
  const testParams = {
    catalogName: 'custom-catalogsource',
    catalogNamespace: 'openshift-marketplace',
    testNamespace: 'test-54307',
    suggestedNamespace: 'testxi3210',
    suggestedNamespaceLabels: 'foo:testxi3120',
    suggestedNamespaceannotations: 'baz:testxi3120'
  }

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI('oc create -f ./fixtures/operators/custom-catalog-source.json')
      .its('stdout')
      .should('contain', 'created');
    cy.checkCommandResult(`oc get catalogsource custom-catalogsource -n openshift-marketplace -o jsonpath='{.status.connectionState.lastObservedState}'`, 'READY');
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc delete CatalogSource custom-catalogsource -n openshift-marketplace`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI('oc delete sub kiali -n openshift-operators');
    cy.adminCLI(`oc delete csv kiali-operator.v1.83.0 -n openshift-operators`);
  });

  it('(OCP-45874,yapei,UserInterface) Check source labels on the operator hub page tiles',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    const queryCatalogSource = `oc get catalogsource custom-catalogsource -n openshift-marketplace -o jsonpath={.status.connectionState.lastObservedState}`;
    cy.checkCommandResult(queryCatalogSource, 'READY', { retries: 6, interval: 10000 }).then(() => {
      Pages.gotoOperatorHubPage();
      operatorHubPage.checkCustomCatalog(OperatorHubSelector.CUSTOM_CATALOG);
      OperatorHubSelector.SOURCE_MAP.forEach((operatorSource, operatorSourceLabel) => {
        operatorHubPage.checkSourceCheckBox(operatorSourceLabel);
        operatorHubPage.getAllTileLabels().each(($el, index, $list) => {
          cy.wrap($el).should('have.text',operatorSource)
        })
        operatorHubPage.uncheckSourceCheckBox(operatorSourceLabel);
      });
    });
  });

  it('(OCP-54544,yapei,UserInterface) Check OperatorHub filter to use nodeArchitectures instead of GOARCH',{tags:['@userinterface','@e2e','admin','@osd-ccs']}, () => {
    // in ocp54544--catalogsource, we have
    // etcd: operatorframework.io/arch.arm64: supported only
    // argocd: didn't define operatorframework.io in CSV, but by default operatorframework.io/arch.amd64 will be added
    // infinispan: for all archs
    const allOperatorsList = ['infinispan','argocd', 'etcd'];
    let includedOperatorsList = ['infinispan'];
    let excludedOperatorsList = [];
    Pages.gotoOperatorHubPage();
    operatorHubPage.checkSourceCheckBox("custom-auto-source");
    cy.adminCLI(`oc get node --selector node-role.kubernetes.io/worker= --show-labels`).then((result) =>{
      if(result.stdout.search('kubernetes.io/arch=arm64') != -1) includedOperatorsList.push('etcd');
      if(result.stdout.search('kubernetes.io/arch=amd64') != -1) includedOperatorsList.push('argocd');
      excludedOperatorsList = allOperatorsList.filter(item => !includedOperatorsList.includes(item));
      cy.log('check operators that should exist');
      includedOperatorsList.forEach((item)=>{
        operatorHubPage.filter(item);
        cy.contains('No Results Match the Filter Criteria').should('not.exist');
        cy.contains('1 item').should('exist');
      })
      cy.log('check operators that should not exist');
      excludedOperatorsList.forEach((item)=>{
        operatorHubPage.filter(item);
        cy.contains('No Results Match the Filter Criteria').should('exist');
      })
    });
  });

  it('(OCP-74621,yapei,UserInterface)Show deprecated operators in OperatorHub)',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    const operator_name_threescale = '3scale-community';
    const deprecated_channel_threescale = 'threescale-2.11';
    const deprecated_version_threescale = '0.8.2';
    const deprecation_msg_version_threescale = "3scale-community-operator.v0.8.2 is deprecated.  Please upgrade to 3scale-community-operator.v0.9.0 or later.";
    Pages.gotoOperatorHubPage();
    operatorHubPage.checkSourceCheckBox("custom-auto-source");
    operatorHubPage.filter(operator_name_threescale);
    operatorHubPage.checkDeprecationLabel('not.exist');
    operatorHubPage.clickOperatorTile(operator_name_threescale);
    operatorHubModal.selectChannel(deprecated_channel_threescale);
    operatorHubModal.selectVersion(deprecated_version_threescale);
    operatorHubPage.checkDeprecationMsg(deprecation_msg_version_threescale.slice(0,20));
    operatorHubPage.checkDeprecationIcon().should('have.length', 2);
    operatorHubModal.clickInstall();
    operatorHubPage.checkDeprecationMsg(deprecation_msg_version_threescale.slice(0,20));
    operatorHubPage.checkDeprecationIcon().should('have.length', 2);
    operatorHubPage.cancel();

    const operator_name_kiali = 'kiali';
    const operator_csv_name = 'Kiali Community Operator';
    const deprecated_channel_kiali = 'alpha';
    const deprecation_msg_package_kiali = "package kiali is end of life.  Please use 'kiali-new' package for support";
    const deprecation_msg_channel_kiali = "channel alpha is no longer supported.  Please switch to channel 'stable'";
    Pages.gotoOperatorHubPage();
    operatorHubPage.checkSourceCheckBox("custom-auto-source");
    // Deprecation label&icon will be shown on OperatorHub list page when package is deprecated
    operatorHubPage.filter(operator_name_kiali);
    operatorHubPage.checkDeprecationLabel('exist');
    // Deprecation label&icon and message was shown on Operator Details modal when when any package, channel or version is deprecated
    operatorHubPage.clickOperatorTile(operator_name_kiali);
    operatorHubPage.checkDeprecationLabel('exist');
    operatorHubPage.checkDeprecationMsg(deprecation_msg_package_kiali.slice(0,20));
    operatorHubModal.selectChannel(deprecated_channel_kiali);
    operatorHubPage.checkDeprecationMsg(deprecation_msg_channel_kiali.slice(0,20));
    operatorHubModal.clickInstall();
    // Deprecation label&icon and message was shown on Operator Installation page when any package, channel or version is deprecated
    operatorHubPage.checkDeprecationLabel('exist');
    operatorHubPage.checkDeprecationMsg(deprecation_msg_package_kiali.slice(0,20));
    operatorHubPage.checkDeprecationMsg(deprecation_msg_channel_kiali.slice(0,20));
    operatorHubPage.clickOperatorInstall();
    // Deprecation label&icon is shown on Installed Operators list page
    cy.checkCommandResult('oc get csv -n openshift-operators', 'kiali-operator');
    Pages.gotoInstalledOperatorPage('openshift-operators');
    operatorHubPage.checkDeprecationLabel('exist');
    installedOperators.clickCSVName(operator_csv_name);
    // Deprecation label&icon and message was shown on Subscription tab
    detailsPage.selectTab('Subscription');
    operatorHubPage.checkDeprecationMsg(deprecation_msg_package_kiali.slice(0,20));
    operatorHubPage.checkDeprecationMsg(deprecation_msg_channel_kiali.slice(0,20));
    // Deprecation icon is shown in channel update modal
    cy.get('.co-detail-table__row .co-detail-table__section').first().within(() => {
      cy.get('button').contains(deprecated_channel_kiali).click();
    });
    cy.get('form.modal-content').within(() => {
      cy.get('svg[class*="yellow-exclamation-icon"]').its('length').should('equal', 1);
    });
    operatorHubPage.cancel();
  });

  it('(OCP-55684,xiyuzhao,UserInterface) Allow operator to specitfy where to run with CSV suggested namespace template annotation',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    cy.visit(`operatorhub/subscribe?pkg=flux-operator&catalog=${testParams.catalogName}&catalogNamespace=${testParams.catalogNamespace}&targetNamespace=undefined`)
      .get('[data-test-id="resource-title"]')
      .should('contain.text','Install Operator')
    cy.get('[data-test="Operator recommended Namespace:-radio-input"]')
      .should('have.value', testParams.suggestedNamespace)
      .should('be.checked');
    // cy.contains(`${testParams.suggestedNamespace} (Operator recommended)`).should('exist')
    cy.contains(`${testParams.suggestedNamespace} does not exist and will be created`).should('exist')
    cy.get('[data-test="install-operator"]').click()

    cy.visit('/k8s/cluster/projects')
    listPage.filter.byName(`${testParams.suggestedNamespace}`)
    listPage.rows.shouldExist(`${testParams.suggestedNamespace}`)
    cy.exec(`oc get project ${testParams.suggestedNamespace} -o template --template={{.metadata}} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false })
      .its('stdout')
      .should('contain',`${testParams.suggestedNamespaceLabels}`)
      .and('contain',`${testParams.suggestedNamespaceannotations}`)
    cy.adminCLI(`oc delete project ${testParams.suggestedNamespace}`);
  });

  it('(OCP-42671,xiyuzhao,UserInterface) OperatorHub shows correct operator installation states',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']},  () => {
    const params ={
      ns: 'test-42671',
      operatorName: 'infinispan-operator',
      csvName: 'Infinispan Operator'
    }
    cy.cliLogin();
    cy.createProjectWithCLI(params.ns);
    operatorHubPage.installOperator(params.operatorName, testParams.catalogName,params.ns);
    Pages.gotoInstalledOperatorPage(params.ns)
    operatorHubPage.checkOperatorStatus(params.csvName, 'Succeeded')
    Pages.gotoOperatorHubPage(params.ns)
    operatorHubPage.checkInstallStateCheckBox('installed')
    operatorHubPage.filter('infinispan');
    cy.get('[class*="card__footer"] span').should('contain.text', "Installed");
    cy.get(`[data-test*="infinispan"]`)
      .should('have.attr','href')
      .then((href) => {
        cy.visit(href);
        cy.byLegacyTestID('operator-uninstall-btn').should('exist');
      });
    cy.get('[data-test-id="operator-modal-box"]').contains('has been installed');
    cy.get('[data-test-id="operator-modal-box"] p a')
      .contains('View it here')
      .should('have.attr','href')
      .then((href) => {
        cy.visit(href);
        cy.byLegacyTestID('horizontal-link-Details').should('exist')
      });
    cy.adminCLI(`oc delete project ${params.ns}`);
  });

  it('(OCP-54037,yapei,UserInterface) Affinity definition support',{tags:['@userinterface','@e2e','admin','@osd-ccs']}, ()=> {
    cy.createProject(testParams.testNamespace);
    operatorHubPage.installOperator('sonarqube-operator', `${testParams.catalogName}`, `${testParams.testNamespace}`);
    Pages.gotoInstalledOperatorPage(testParams.testNamespace)
    operatorHubPage.checkOperatorStatus('Sonarqube Operator', 'Installing');
    cy.visit(`/k8s/ns/${testParams.testNamespace}/operators.coreos.com~v1alpha1~ClusterServiceVersion/sonarqube-operator.v0.0.6/sonarsource.parflesh.github.io~v1alpha1~SonarQube`)
    cy.byTestID('item-create').click();
    Operand.switchToFormView();
    // set required values
    Operand.expandNodeConfigAdvanced();
    Operand.clickAddNodeConfigAdvanced();
    Operand.setRandomType()
    // set values for nodeAffinity
    Operand.expandNodeAffinity();
    Operand.nodeAffinityAddRequired('topology.kubernetes.io/zone', 'In', 'antarctica-east1,antarctica-east2');

    Operand.nodeAffinityAddPreferred('1','another-node-label-key', 'In', 'another-node-label-value');
    Operand.collapseNodeAffinity();
    // set values for podAffinity
    Operand.expandPodAffinity();
    Operand.podAffinityAddRequired('topology.kubernetes.io/zone','security', 'In', 'S1');
    Operand.collapsePodAffinity();
    // set values for podAntiAffinity
    Operand.expandPodAntiAffinity();
    Operand.podAntiAffinityAddPreferred('100','topology.kubernetes.io/zone','security', 'In', 'S2');
    Operand.collapsePodAntiAffinity();
    Operand.submitCreation();
    cy.wait(10000);
    cy.adminCLI(`oc get sonarqube.sonarsource.parflesh.github.io -n ${testParams.testNamespace} -o yaml`)
      .its('stdout')
      .should('contain', 'example-sonarqube')
      .and('contain', '- antarctica-east1')
      .and('contain','- antarctica-east2')
      .and('contain','- S1')
      .and('contain','- S2')
    cy.adminCLI(`oc delete project ${testParams.testNamespace}`);
  });

  it('(OCP-62266,xiyuzhao,UserInterface)  Filter operators based on nodes OS type',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    let nodeOS;
    const checkFilterResult = (operator: string, state: string) =>{
      operatorHubPage.filter(operator);
      cy.contains("No Results Match the Filter Criteria").should(state);
    }
    cy.exec(`oc get nodes -o yaml -o jsonpath={.items[*].status.nodeInfo.operatingSystem} \
                --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | \
             xargs -n 1 | \
             uniq`, { failOnNonZeroExit: false })
      .then((result) => {
        return nodeOS = result.stdout;
        cy.log(result.stdout);
        cy.log(result.stderr);
    });
    /* Aqua operator has label operatorframework.io/os.windows: supported
        which means it will only shown on OperatorHub page when node os has windows type */
    Pages.gotoOperatorHubPage();
    operatorHubPage.checkSourceCheckBox("custom-auto-source");
    cy.wrap(nodeOS).then(()=> {
      if(nodeOS.includes('windows')) {
        checkFilterResult('aqua','not.exist');
        cy.contains('1 item').should('exist');
      }else{
        checkFilterResult('aqua','exist');
      }
    });
    //Check Server_Flags and configmaps has nodeOperatingSystems
    cy.window().then((win: any) => {
      let opt = win.SERVER_FLAGS.nodeOperatingSystems;
      expect(opt).contain(nodeOS);
    });
    cy.exec(`oc get configmaps console-config -n openshift-console -o yaml \
                --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | \
             awk '$1 == "-"{ if (key == "nodeOperatingSystems:") print $NF; next } {key=$1}' \
             `, { failOnNonZeroExit: false })
      .then((output) => {
        expect(output.stdout).to.include(nodeOS);
    })
  });

  it('(OCP-71516,xiyuzhao,UserInterface) Add TLSProfiles and tokenAuthGCP annotation to Infrastructures features filter section',{tags:['@userinterface','@e2e','admin']}, function () {
    cy.checkClusterType('isGCPCluster').then(value => {
      if (value === false) {
        cy.log('This is not a GCP Platform, Skip the case!!');
        this.skip();
      }
    })
    // Check the new annotation is listed on the Infrastructure filter list
    Pages.gotoOperatorHubPage();
    operatorHubPage.checkInfraFeaturesCheckbox("configurable-tls-ciphers");
    operatorHubPage.checkInfraFeaturesCheckbox("auth-token-gcp");
    // Check the annotation is added for the Operator
    cy.visit('/operatorhub/all-namespaces?details-item=kiali-operator-custom-catalogsource-openshift-marketplace')
    cy.contains('h5', 'Infrastructure features')
      .parent()
      .within(() => {
        cy.contains('div', 'Auth Token GCP').should('exist');
        cy.contains('div', 'Configurable TLS ciphers').should('exist');
      });
  });
})
