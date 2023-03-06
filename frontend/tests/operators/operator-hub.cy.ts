import {operatorHubPage, OperatorHubSelector} from "../../views/operator-hub-page";
import { listPage } from "upstream/views/list-page";

describe('Operator Hub tests', () => {
    const testParams = {
        catalogName: 'custom-catalogsource',
        catalogNamespace: 'openshift-marketplace',
        suggestedNamespace: 'testxi3210',
        suggestedNamespaceLabels: 'foo:testxi3120',
        suggestedNamespaceannotations: 'baz:testxi3120'
      }
    
    before(() => {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
        cy.adminCLI(`oc create -f ./fixtures/operators/custom-catalog-source.json`);
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    });

    after(() => {
        cy.adminCLI(`oc delete CatalogSource custom-catalogsource -n openshift-marketplace`);
        cy.adminCLI(`oc delete project ${testParams.suggestedNamespace}`);
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    });

    it('(OCP-45874) Check source labels on the operator hub page tiles', {tags: ['e2e','admin']}, () => {
        operatorHubPage.goTo()
        operatorHubPage.checkCustomCatalog(OperatorHubSelector.CUSTOM_CATALOG)
        OperatorHubSelector.SOURCE_MAP.forEach((operatorSource, operatorSourceLabel) => {
            operatorHubPage.checkSourceCheckBox(operatorSourceLabel)
            operatorHubPage.getAllTileLabels()
                .each(($el, index, $list) => {
                cy.wrap($el).should('have.text',operatorSource)
            })
            operatorHubPage.uncheckSourceCheckBox(operatorSourceLabel)
        });
    });

    it('(OCP-54544,yapei) Check OperatorHub filter to use nodeArchitectures instead of GOARCH', {tags: ['e2e','admin','@osd-ccs']}, () => {
        // in ocp54544--catalogsource, we have 
        // etcd: operatorframework.io/arch.arm64: supported only  
        // argocd: didn't define operatorframework.io in CSV, but by default operatorframework.io/arch.amd64 will be added 
        // infinispan: for all archs
        const allOperatorsList = ['infinispan','argocd', 'etcd'];
        let includedOperatorsList = ['infinispan'];
        let excludedOperatorsList = [];
        operatorHubPage.goTo();
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
  
    it('(OCP-55684, xiyuzhao) Allow operator to specitfy where to run with CSV suggested namespace template annotation', {tags: ['e2e','admin']}, () => {
        cy.visit(`operatorhub/subscribe?pkg=flux-operator&catalog=${testParams.catalogName}&catalogNamespace=${testParams.catalogNamespace}&targetNamespace=undefined`)
          .get('[data-test-id="resource-title"]')
          .should('contain.text','Install Operator')
        cy.contains(`${testParams.suggestedNamespace} (Operator recommended)`).should('exist')
        cy.contains(`${testParams.suggestedNamespace} does not exist and will be created`).should('exist')
        cy.get('[data-test="install-operator"]').click()

        cy.visit('/k8s/cluster/projects')
        listPage.filter.byName(`${testParams.suggestedNamespace}`)
        listPage.rows.shouldExist(`${testParams.suggestedNamespace}`)
        cy.exec(`oc get project ${testParams.suggestedNamespace} -o template --template={{.metadata}} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false })
          .its('stdout')
          .should('contain',`${testParams.suggestedNamespaceLabels}`)
          .and('contain',`${testParams.suggestedNamespaceannotations}`)        
    })
})
