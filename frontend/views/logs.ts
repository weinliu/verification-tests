export const logsPage = {
  logLinesNotContain: (lines: string) => cy.get('[class*=log-viewer__text]', {timeout: 6000}).should('not.contain.text', lines),
  logWindowLoaded: () => cy.get('[class*=log-viewer__text]', {timeout: 90000}).should('exist'),
  filterByUnit: (unitname: string) => {
    cy.get('#log-unit').clear();
    cy.get('#log-unit').type(unitname).type('{enter}');
  },
  selectContainer: (containername?, containernumber?) => {
    cy.get('button[data-test-id="dropdown-button"]').click();
    if(containername){
      cy.contains('span.co-resource-item__resource-name', `${containername}`).click();
    }else if(containernumber){
      cy.get('ul.[class*=dropdown__menu] li button').eq(`${containernumber}-1`).click();
    }
  },
  selectLogComponent: (componentname: string) => {
    cy.get('button[class*=select__toggle]').click();
    cy.get('[class*=select__menu-item').contains(componentname).click();
  },
  selectLogFile: (logname: string) => {
    cy.get('button[class*=select__toggle]').last().click();
    cy.get('[class*=select__menu-item]').contains(logname).click();
  },
  checkLogLineExist: () => cy.get('[class*=log-viewer__index]').should('exist'),
  searchLog: (keyword) => {
    cy.get('[class*=log-viewer__scroll-container]', {timeout: 30000}).scrollTo('top', { ensureScrollable: false });
    cy.get('input[placeholder="Search"]').type(`${keyword}`);
    cy.get('span[class*=log-viewer__string]', {timeout: 60000}).contains(`${keyword}`, { matchCase: false });
  },
  clearSearch: () => {
    cy.get('[aria-label="Reset"]').click();
    cy.get('input[placeholder="Search"]').should("have.attr", "value", "");
  },
  checkLogWraped: (boolvalue) => {
    cy.get('#wrapLogLines').should('have.attr', 'data-checked-state', `${boolvalue}`);
  },
  setLogWrap: (boolvalue) => {
    cy.get('#wrapLogLines').then(($elem) => {
      const $checkedstate = $elem.attr('data-checked-state');
      cy.log($checkedstate);
      if(boolvalue != $checkedstate){
        cy.contains('Wrap lines').click();
      }
    })
  }
}
