import * as yamlEditor from '../upstream/views/yaml-editor'

export const importYamlPage = {
    dragDropYamlFile: (importfile: string) => {
      yamlEditor.clickSaveCreateButton();
      cy.get('.ocs-yaml-editor__root')
        .selectFile(importfile, {action: 'drag-drop'});
    },
    checkDangerAlert: (alertmsg: RegExp) => {
      cy.contains(/error|Danger|alert/gi).should('exist');
      cy.contains(alertmsg).should('exist');
    },
    open: () => {
      cy.get('button[data-test="quick-create-dropdown"]').click();
      cy.get('li[data-test="qc-import-yaml"] a').click();
    }
  }
export const yamlOptions = {
  setTooltips: (action: string) => {
    cy.get("input[id='showTooltips']").then(($elem) => {
    const checkedstate = $elem.attr('data-checked-state');
    if(checkedstate === 'true'){
      cy.log('the "Show tooltips" input is currently checked');
      if(action === 'show'){
        cy.log('nothing to do since it already checked')
      } else if(action === 'hide') {
        cy.log('uncheck "Show tooltips"');
        cy.get("input[id='showTooltips']").click();
      }
    } else if (checkedstate === 'false') {
      cy.log('the "Show tooltips" input is currently not checked');
      if(action === 'show'){
        cy.log('check "Show tooltips"');
        cy.get("input[id='showTooltips']").click();
      } else if(action === 'hide') {
        cy.log('nothing to do since it already un-checked')
      }
    }
    })
  },
  checkTooltipsVisibility: (schemaName, tooltipsContext, visibility) => {
    const match = visibility === 'shown' ? 'be.visible' : 'not.be.visible';
    cy.contains('span.mtk26', `${schemaName}`).click();
    cy.contains(`${tooltipsContext}`).should(`${match}`);
  }
}
