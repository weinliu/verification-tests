import {operatorHubPage, OperatorHubSelector} from "../../views/operator-hub-page";
import customCatalog from '../../fixtures/custom-catalog-source.json'

describe('Operator Hub tests', () => {
    before(() => {
        cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`echo '${JSON.stringify(customCatalog)}' | oc create -f - --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    });

    after(() => {
        cy.exec(`oc delete CatalogSource custom-catalogsource -n openshift-marketplace --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
        cy.logout;
    });

    it('(OCP-45874) Check source labels on the operator hub page tiles', () => {
        operatorHubPage.goTo()
        operatorHubPage.isLoaded()
        operatorHubPage.checkCustomCatalog(OperatorHubSelector.CUSTOM_CATALOG)
        OperatorHubSelector.SOURCE_MAP.forEach((operatorSource, operatorSourceLabel) => {
            operatorHubPage.checkSourceCheckBox(operatorSourceLabel)
            operatorHubPage.getAllTileLabels()
                .each(($el, index, $list) => {
                cy.wrap($el).should('have.text',operatorSource)
            })
            operatorHubPage.uncheckSourceCheckBox(operatorSourceLabel)
        })
    });
})
