export const logsPage = {
  logLinesNotContain: (lines: string) => cy.get('[class*=log-viewer__text]', {timeout: 6000}).should('not.contain.text', lines),
  logWindowLoaded: () => cy.get('[class*=log-viewer__text]', {timeout: 90000}).should('exist'),
  filterByUnit: (unitname: string) => {
    cy.get('#log-unit').clear();
    cy.get('#log-unit').type(unitname).type('{enter}');
  },
  selectContainer: (containername) => {
    cy.get('button[data-test="container-select"]').click();
    cy.contains('span', `${containername}`).parent().parent().parent().parent('button[role="option"]').click();
  },
  selectLogComponent: (componentname: string) => {
    cy.get('button[data-test="select-path"]').click();
    cy.get('span').contains(componentname).parentsUntil('button[role="option"]').click();
  },
  selectLogFile: (logname: string) => {
    cy.get('button[data-test="select-file"]').click();
    cy.get('span').contains(logname).parentsUntil('button[role="option"]').click();
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
        cy.contains('Wrap lines').click({force: true});
      }
    })
  }
}
