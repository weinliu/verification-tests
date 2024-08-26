import {listPage} from "../upstream/views/list-page";

export const nnsPage = {
    goToNNS: () => {
        cy.contains('Networking').should('be.visible');
        cy.clickNavLink(['Networking', 'NodeNetworkState']);
    },
    filterByRequester: (selector) => {
        listPage.filter.clickFilterDropdown();
        cy.get('#'+selector).check();
    },
    searchByRequester: (selectorKey, selectorVal) => {
        listPage.filter.clickSearchByDropdown();
        cy.get(`button[data-test-dropdown-menu="${selectorKey}"]`).click();
        cy.byLegacyTestID('item-filter').type(selectorVal);
    },
    checkNNS: (nodeList, isExist, intType, intName?, intIP?) => {
        nnsPage.goToNNS();
        nodeList.forEach((node) => {
            if (isExist) {
                cy.byTestID(node).parents('tbody').within(() => {
                    cy.get('td[id="network-interface"]').contains(intType).click();
                });
                cy.get('div[role="dialog"]').contains(intName).should('exist');
                if (intIP) {
                    cy.get('div[role="dialog"]').contains(intIP).should('exist');
                };
                cy.get('div[role="dialog"]').get('[aria-label="Close"]').last().click();
            } else {
                let cmd = "";
                let resultKey = "";
                if (intType == "linux-bridge") {
                    cmd = `oc debug node/${node} -- chroot /host bash -c nmcli -f TYPE dev | grep bridge | grep -v ovs`;
                    resultKey = "bridge"
                };
                if (intType == "bond") {
                    cmd = `oc debug node/${node} -- chroot /host bash -c nmcli -f TYPE dev | grep bond`;
                    resultKey = "bond"
                };
                if (cmd != "") {
                    cy.exec(cmd, {failOnNonZeroExit: false}).then(result => {
                        if(result.stdout.includes(resultKey)){
                            cy.byTestID(node).parents('tbody').within(() => {
                                cy.get('td[id="network-interface"]').contains(intType).click();
                            });
                            cy.get('div[role="dialog"]').contains(intName).should('not.exist');
                            cy.get('div[role="dialog"]').get('[aria-label="Close"]').last().click();
                        } else {
                            cy.byTestID(node).parents('tbody').within(() => {
                                cy.get('td[id="network-interface"]').contains(intType).should('not.exist');
                            });
                        };
                    });
                } else {
                    cy.byTestID(node).parents('tbody').within(() => {
                        cy.get('td[id="network-interface"]').contains(intType).should('not.exist');
                    });
                };
            };
        });
    },
};
