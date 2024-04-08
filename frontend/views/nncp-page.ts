export interface intPolicy {
    intName: string,
    intState: string,
    intType: string,
    ipv4Enable: boolean,
    ip: string,
    prefixLen: string,
    disAutoDNS: boolean,
    disAutoRoutes: boolean,
    disAutoGW: boolean,
    port: string,
    br_stpEnable: boolean,
    bond_cpMacFrom: string,
    bond_aggrMode: string,
    action: string,
};

export const nncpPage = {
    goToNNCP: () => {
        cy.contains('Networking').should('be.visible');
        cy.clickNavLink(['Networking', 'NodeNetworkConfigurationPolicy']);
    },
    addPolicy:(policy) => {
        cy.get('#policy-interface-name-0').clear().type(policy.intName);
        cy.get('#policy-interface-network-state-0').select(policy.intState);
        cy.get('#policy-interface-type-0').select(policy.intType);
        if (policy.ipv4Enable) {
            cy.get('#policy-interface-ip-0').check();
            if (policy.ip) {
                cy.get('#ip-0').click();
                cy.get('#ipv4-address-0').clear().type(policy.ip);
                cy.get('[id="prefix-length-0"]').within(() => {
                    cy.get('input[type="number"]').type('{selectAll}'+policy.prefixLen);
                });
            } else {
                cy.get('#dhcp-0').click();
                if (policy.disAutoDNS) { cy.get('#policy-interface-dns-0').uncheck(); }
                if (policy.disAutoRoutes) { cy.get('#policy-interface-routes-0').uncheck(); }
                if (policy.disAutoGW) { cy.get('#policy-interface-gateway-0').uncheck(); }
            };
        };
        if (policy.port) { cy.get('#policy-interface-port-0').clear().type(policy.port); }
        if (policy.intType == "Bridge") {
            if (policy.br_stpEnable) { cy.get('#policy-interface-stp-0').check(); }
        };
        if (policy.intType == "Bonding") {
            if (policy.bond_cpMacFrom) {
                cy.byLegacyTestID('copy-mac-0').within(() => { cy.byButtonText('Copy MAC address').click(); })
                cy.get('#policy-interface-copy-mac-from-0').clear().type(policy.bond_cpMacFrom);
            };
            if (policy.bond_aggrMode) {
                cy.get('#policy-interface-aggregation-0').select(policy.bond_aggrMode);
            };
        };
    },
    cfgSelector:(selectorKey, selectorVal) => {
        cy.get('#apply-nncp-selector').check();
        cy.byButtonText('Add Label').click();
        cy.get('#label-0-key-input').clear().type(selectorKey);
        if (selectorVal) { cy.get('#label-0-value-input').clear().type(selectorVal); };
        cy.get('button[type="submit"]').contains('Save').click();
    },
    creatNNCPFromForm: (policyName, desc, intPolicyList:Array<intPolicy>, selectorKey?, selectorVal?) => {
        nncpPage.goToNNCP();
        cy.byTestID('item-create').click();
        cy.byTestID('list-page-create-dropdown-item-form').click();
        cy.get('#policy-name').clear().type(policyName);
        cy.get('#policy-description').clear().type(desc);

        for (let i = 0; i < intPolicyList.length; i++) {
            if (i >= 1) {
                cy.get('p[class="policy-form-content__add-new-interface pf-u-mt-md"]').within(() => {
                    cy.get('button[type="button"]').click();
                });
            };
            let policy = intPolicyList[i];
            nncpPage.addPolicy(policy);
        };
        if (selectorKey) {
            nncpPage.cfgSelector(selectorKey, selectorVal);
        };
        cy.get('button[form="create-policy-form"]').click();
    },
    editNNCPFromForm: (policyName, desc, intPolicyList:Array<intPolicy>) => {
        nncpPage.goToNNCP();
        cy.get(`[data-test-rows="resource-row"]`).contains(policyName).parents('tr').within(() => {
            cy.get(`[class='pf-v5-c-menu-toggle pf-m-plain']`).click();
        });
        cy.get(':nth-child(1) > .pf-v5-c-menu__item').click();

        cy.get('#policy-description').clear().type(desc);
        //if there is new interface policies, add them at first
        for (let i = 0; i < intPolicyList.length; i++) {
            let policy = intPolicyList[i]
            if (policy.action == "addNew") {
                cy.get('p[class="policy-form-content__add-new-interface pf-u-mt-md"]').within(() => {
                    cy.get('button[type="button"]').click();
                });
                nncpPage.addPolicy(policy);
                policy.action = "";
            };
        };
        for (let i = 0; i < intPolicyList.length; i++) {
            let policy = intPolicyList[i];
            if (policy.action == "editOld") {
                let j = intPolicyList.length - i - 1;
                cy.get('#policy-interface-network-state-'+j).select(policy.intState);
                if (policy.ipv4Enable) {
                    cy.get('#policy-interface-ip-'+j).check();
                    if (policy.ip) {
                        cy.get('#ip-'+j).click();
                        cy.get('#ipv4-address-'+j).clear().type(policy.ip);
                        cy.get('[id="prefix-length-'+j+'"]').within(() => {
                            cy.get('input[type="number"]').type('{selectAll}'+policy.prefixLen);
                        });
                    } else {
                        cy.get('#dhcp-'+j).click();
                        if (policy.disAutoDNS) {
                            cy.get('#policy-interface-dns-'+j).uncheck();
                        } else {
                            cy.get('#policy-interface-dns-'+j).check();
                        };
                        if (policy.disAutoRoutes) {
                            cy.get('#policy-interface-routes-'+j).uncheck();
                        } else {
                            cy.get('#policy-interface-routes-'+j).check();
                        };
                        if (policy.disAutoGW) {
                            cy.get('#policy-interface-gateway-'+j).uncheck();
                        } else {
                            cy.get('#policy-interface-gateway-'+j).check();
                        };
                    };
                } else {
                    cy.get('#policy-interface-ip-'+j).uncheck();
                };
                if (policy.port) { cy.get('#policy-interface-port-'+j).clear().type(policy.port); }
                if (policy.intType == "Bonding") {
                    if (policy.bond_cpMacFrom) {
                        cy.byLegacyTestID('copy-mac-'+j).within(() => { cy.byButtonText('Copy MAC address').click(); })
                        cy.get('#policy-interface-copy-mac-from-'+j).clear().type(policy.bond_cpMacFrom);
                    };
                    if (policy.bond_aggrMode) {
                        cy.get('#policy-interface-aggregation-'+j).select(policy.bond_aggrMode);
                    };
                };
                policy.action = "";
            };
        };
        cy.get('button[type="submit"]').click();
    },
    deleteNNCP: (policyName) => {
        nncpPage.goToNNCP();
        cy.get(`[data-test-rows="resource-row"]`).contains(policyName).parents('tr').within(() => {
            cy.get(`[class='pf-v5-c-menu-toggle pf-m-plain']`).click();
        });
        cy.get(':nth-child(2) > .pf-v5-c-menu__item').click();
        cy.get('body').then($body => {
            cy.get('#text-confirmation').clear().type(policyName)
            cy.get('button[type="submit"]').click()
        })
    },
};
