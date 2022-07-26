
export const netflowPage = {
    visit: () => {
        cy.visit('/netflow-traffic')

        // set the page to auto refresh
        cy.byTestID(genSelectors.refreshDrop).then(btn => {
            expect(btn).to.exist
            cy.wrap(btn).click().then(drop => {
                cy.get('[data-test="15s"]').should('exist').click()
            })
        })

        cy.byTestID("table-composable").should('exist')
    },
}

export namespace genSelectors {
    export const timeDrop = "time-range-dropdown-dropdown"
    export const refreshDrop = "refresh-dropdown-dropdown"
    export const refreshBtn = 'refresh-button'
    export const moreOpts = 'more-options-button'
    export const FullScreen = 'fullscreen-button'
    export const CSVExport = 'export-button'
    export const compact = '[index="0"] > ul > :nth-child(1) > .pf-c-dropdown__menu-item'
    export const normal = ':nth-child(2) > .pf-c-dropdown__menu-item'
    export const large = ':nth-child(3) > .pf-c-dropdown__menu-item'
    export const exportCsv = '[index="1"] > ul > li > .pf-c-dropdown__menu-item'
    export const expand = '[index="2"] > ul > li > .pf-c-dropdown__menu-item'
}

export namespace colSelectors {
    export const mColumns = 'manage-columns-button'
    export const columnsModal = '.modal-content'

    export const save = 'columns-save-button'
    export const resetDefault = 'columns-reset-button'

    export const Mac = '[data-test=th-Mac] > .pf-c-table__button'
    export const gK8sOwner = '[data-test=th-K8S_OwnerObject] > .pf-c-table__button > .pf-c-table__button-content > .pf-c-table__text'
    export const gIPPort = '[data-test=th-AddrPort] > .pf-c-table__button > .pf-c-table__button-content > .pf-c-table__text'
    export const Protocol = '[data-test=th-Proto] > .pf-c-table__button'

    export const srcNodeIP = '[data-test=th-SrcK8S_HostIP] > .pf-c-table__button > .pf-c-table__button-content > .pf-c-table__text'
    export const srcNS = '[data-test=th-SrcK8S_Namespace] > .pf-c-table__button > .pf-c-table__button-content > .pf-c-table__text'

    export const dstNodeIP = '[data-test=th-DstK8S_HostIP] > .pf-c-table__button'
    export const direction = '[data-test=th-FlowDirection] > .pf-c-table__button'
    export const bytes = '[data-test=th-Bytes] > .pf-c-table__button > .pf-c-table__button-content > .pf-c-table__text'

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
