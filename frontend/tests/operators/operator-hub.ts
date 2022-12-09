import {operatorHubPage, OperatorHubSelector} from "../../views/operator-hub-page";

describe('Operator Hub tests', () => {
    before(() => {
        cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`oc create -f ./fixtures/operators/custom-catalog-source.json --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    });

    after(() => {
        cy.exec(`oc delete CatalogSource custom-catalogsource -n openshift-marketplace --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
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

    it('(OCP-54544,yapei) Check OperatorHub filter to use nodeArchitectures instead of GOARCH', {tags: ['e2e','admin']}, () => {
        // in ocp54544--catalogsource, we have 
        // etcd: operatorframework.io/arch.arm64: supported only  
        // argocd: didn't define operatorframework.io in CSV, but by default operatorframework.io/arch.amd64 will be added 
        // infinispan: for all archs
        const allOperatorsList = ['infinispan','argocd', 'etcd'];
        let includedOperatorsList = ['infinispan'];
        let excludedOperatorsList = [];
        operatorHubPage.goTo();
        operatorHubPage.checkSourceCheckBox("custom-auto-source");
        cy.exec(`oc get node --selector node-role.kubernetes.io/worker= --show-labels --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`).then((result) =>{
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
    })
})
