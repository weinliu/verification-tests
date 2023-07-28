
export const netflowPage = {
    visit: () => {
        cy.intercept('**/backend/api/loki/topology*').as('call1')
        cy.visit('/netflow-traffic')
        // wait for all calls to complete before checking due to bug
        cy.wait('@call1', { timeout: 60000 }).wait('@call1')

        netflowPage.clearAllFilters()

        // set the page to auto refresh
        cy.byTestID(genSelectors.refreshDrop).then(btn => {
            expect(btn).to.exist
            cy.wrap(btn).click().then(drop => {
                cy.get('[data-test="15s"]').should('exist').click()
            })
        })
        cy.byTestID('no-results-found').should('not.exist')
        cy.get('#overview-container').should('exist')
    },
    toggleFullScreen: () => {
        cy.byTestID(genSelectors.moreOpts).should('exist').click().then(moreOpts => {
            cy.get(genSelectors.expand).click()
        })
    },
    stopAutoRefresh: () => {
        cy.byTestID(genSelectors.refreshDrop).then(btn => {
            expect(btn).to.exist
            cy.wrap(btn).click().then(drop => {
                cy.get('[data-test="OFF_KEY"]').should('exist').click()
            })
        })
    },
    resetClearFilters: () => {
        cy.get('#chips-more-options-dropdown').should('exist').click().then(moreOpts => {
            cy.contains("Reset defaults").should('exist').click()
        })
    },
    clearAllFilters: () => {
        cy.get('#chips-more-options-dropdown').should('exist').click().then(moreOpts => {
            cy.contains("Clear all").should('exist').click()
        })
    }
}

export const topologyPage = {
    selectScopeGroup: (scope: any, group: any) => {
        cy.contains('Display options').should('exist').click()
        if (scope) {
            cy.byTestID("scope-dropdown").click().byTestID(scope).click()
        }
        if (group) {
            cy.wait(5000)
            cy.byTestID("group-dropdown").click().byTestID(group).click()
        }
        cy.contains('Display options').should('exist').click()
    },
    isViewRendered: () => {
        cy.get('[data-surface="true"]').should('exist')
    }
}

export namespace genSelectors {
    export const timeDrop = "time-range-dropdown-dropdown"
    export const refreshDrop = "refresh-dropdown-dropdown"
    export const refreshBtn = 'refresh-button'
    export const moreOpts = 'more-options-button'
    export const FullScreen = 'fullscreen-button'
    export const CSVExport = 'export-button'
    export const exportCsv = '[index="1"] > ul > li > .pf-c-dropdown__menu-item'
    export const expand = '[index="2"] > ul > li > .pf-c-dropdown__menu-item'
}

export namespace colSelectors {
    export const mColumns = '#view-options-dropdown > ul > section:nth-child(1) > ul > li > a'
    export const columnsModal = '.modal-content'
    export const save = 'columns-save-button'
    export const resetDefault = 'columns-reset-button'
    export const Mac = '[data-test=th-Mac] > .pf-c-table__button'
    export const gK8sOwner = '[data-test=th-K8S_OwnerObject] > .pf-c-table__button'
    export const gIPPort = '[data-test=th-AddrPort] > .pf-c-table__button'
    export const Protocol = '[data-test=th-Proto] > .pf-c-table__button'
    export const srcNodeIP = '[data-test=th-SrcK8S_HostIP] > .pf-c-table__button'
    export const srcNS = '[data-test=th-SrcK8S_Namespace] > .pf-c-table__button'
    export const dstNodeIP = '[data-test=th-DstK8S_HostIP] > .pf-c-table__button'
    export const direction = '[data-test=th-FlowDirection] > .pf-c-table__button'
    export const bytes = '[data-test=th-Bytes] > .pf-c-table__button'
    export const packets = '[data-test=th-Packets] > .pf-c-table__button'
}

export namespace filterSelectors {
    export const filterGroupText = '.custom-chip > p'
}

export namespace querySumSelectors {
    export const queryStatsPanel = "#query-summary"
    export const flowsCount = "#flowsCount"
    export const bytesCount = "#bytesCount"
    export const packetsCount = "#packetsCount"
    export const bpsCount = "#bpsCount"
    export const expandedQuerySummaryPanel = '.pf-c-drawer__panel-main'
}

export namespace topologySelectors {
    export const metricsDrop = 'metricFunction-dropdown'
    export const metricsList = '#metricFunction > ul > li'
    export const optsClose = '.pf-c-drawer__close > .pf-c-button'
    export const nGroups = '[data-layer-id="groups"] > g'
    export const group = 'g[data-type="group"]'
    export const node = 'g[data-kind="node"]:empty'
    export const edge = 'g[data-kind="edge"]'
    export const groupLayer = '[data-layer-id="groups"]'
    export const defaultLayer = '[data-layer-id="default"]'
    export const groupToggle = '[for="group-collapsed-switch"] > .pf-c-switch__toggle'
    export const edgeToggle = "#edges-switch"
    export const labelToggle = '#edges-tag-switch'
    export const badgeToggle = '#badge-switch'
}

export namespace overviewSelectors {
    export const mPanels = '#view-options-dropdown > ul > section:nth-child(1) > ul > li > a'
    export const panelsModal = '.modal-content'
    export const typeDrop = 'type-dropdown'
    export const scopeDrop = 'scope-dropdown'
    export const truncateDrop = 'truncate-dropdown'
    export const defaultPanels = ['Top 5 average rates', 'Top 5 latest rates', 'Top 5 flow rates stacked with total', 'Top 5 flow rates']
    export const allPanels = ['Top 5 average rates', 'Top 5 latest rates', 'Top 5 flow rates stacked', 'Total rate', 'Top 5 flow rates stacked with total', 'Top 5 flow rates']
}

export namespace histogramSelectors {
    export const timeRangeContainer = "#chart-histogram > div.pf-l-flex.pf-m-row.histogram-range-container"
    export const zoomin = timeRangeContainer + " > div:nth-child(5) > div > div:nth-child(2) > button"
    export const zoomout = timeRangeContainer + "> div:nth-child(5) > div > div:nth-child(1) > button"
    const forwardShift = timeRangeContainer + "> div:nth-child(4)"
    export const singleRightShift = forwardShift + "> button:nth-child(1)"
    export const doubleRightShift = forwardShift + "> button:nth-child(2)"
    const backwardShift = timeRangeContainer + "> div:nth-child(2)"
    export const singleLeftShift = backwardShift + "> button:nth-child(2)"
    export const doubleLeftShift = backwardShift + "> button:nth-child(1)"
}

Cypress.Commands.add('showAdvancedOptions', () => {
    cy.get('#show-view-options-button')
        .then(function ($button) {
            if ($button.text() === 'Hide advanced options') {
                return;
            } else {
                cy.get('#show-view-options-button').click();
            }
        })
});

Cypress.Commands.add('checkPanelsNum', (panels = 4) => {
    cy.get('#overview-flex').find('.overview-card').its('length').should('eq', panels);
});

Cypress.Commands.add('checkPanel', (panelName) => {
    for (let i = 0; i < panelName.length; i++) {
        cy.get('#overview-flex').contains(panelName[i]);
        cy.get('[data-test-metrics]').its('length').should('gt', 0);
    }
});

Cypress.Commands.add('openPanelsModal', () => {
    cy.showAdvancedOptions();
    cy.get('#view-options-button').click();
    cy.get(overviewSelectors.mPanels).click().then(panel => {
        cy.get(overviewSelectors.panelsModal).should('exist')
    })
});

Cypress.Commands.add('checkPopupItems', (id, names) => {
    for (let i = 0; i < names.length; i++) {
        cy.get(id).contains(names[i])
            .closest('.pf-c-data-list__item-row').find('.pf-c-data-list__check');
    }
});

Cypress.Commands.add('selectPopupItems', (id, names) => {
    for (let i = 0; i < names.length; i++) {
        cy.get(id).contains(names[i])
            .closest('.pf-c-data-list__item-row').find('.pf-c-data-list__check').click();
    }
});

Cypress.Commands.add('checkQuerySummary', (metric) => {
    let warningExists = false
    let num = 0
    cy.get(querySumSelectors.queryStatsPanel).should('exist').then(qrySum => {
        if (Cypress.$(querySumSelectors.queryStatsPanel + ' svg.query-summary-warning').length > 0) {
            warningExists = true
        }
    })
    if (warningExists) {
        num = Number(metric.text().split('+ ')[0])
    }
    else {
        num = Number(metric.text().split(' ')[0])
    }
    expect(num).to.be.greaterThan(0)
});

declare global {
    namespace Cypress {
        interface Chainable {
            showAdvancedOptions(): Chainable<Element>
            checkPanelsNum(panels?: number): Chainable<Element>
            checkPanel(panelName: string[]): Chainable<Element>
            openPanelsModal(): Chainable<Element>
            selectPopupItems(id: string, names: string[]): Chainable<Element>
            checkPopupItems(id: string, names: string[]): Chainable<Element>
            checkQuerySummary(metric: JQuery<HTMLElement>): Chainable<Element>
        }
    }
}
