export namespace podsPageUtils {
    export function setProjectPodNamesAlias(project, label, aliasPrefix, pod_label_key = "name") {
      cy.visit(`/k8s/ns/${project}/pods`).get('[data-test-id="resource-title"]').should('be.visible')
      cy.get('#content-scrollable').within(() => {
        cy.get('button.pf-c-dropdown__toggle')
          .should('have.text', 'Name')
          .click()
          .get('#LABEL-link')
          .click()
        cy.byLegacyTestID('item-filter')
          .type(`${pod_label_key}=${label}`)
          .get('span.co-text-node')
          .contains(label)
          .should('be.visible')
          .click()
        cy.get('tr > td[id=name]').find('a').each(($el, $index) => {
          cy.wrap($el).invoke('text').as(`${aliasPrefix}_pod${$index}Name`)
        })
      })
    }
    export function setPodIPAlias(project, podName) {
      cy.visit(`./k8s/ns/${project}/pods/${podName}`)
        .byTestSelector('details-item-value__Pod IP')
        .should('be.visible')
        .invoke('text')
        .as(`${podName}IP`)
    }
  }
  export const podsPage = {
    goToPodsInAllNamespaces: () => {
      cy.visit('/k8s/all-namespaces/pods');
      cy.get('tr[data-test-rows="resource-row"]').should('exist');
    },
    goToPodsForGivenNamespace: (namespace: String) => {
      cy.visit('/k8s/ns/'+namespace+'/pods');
      cy.get('tr[data-test-rows="resource-row"]').should('exist');
    },
    // this is to make sure the page is loaded,
    // the pods page is loaded when the columns are displayed hence checking for this condition
    isLoaded: () => {
      cy.get('tr[data-test-rows="resource-row"]').should('exist')
    },
    goToPodDetails: (namespace, podName) => {
      cy.visit('/k8s/ns/'+namespace+'/pods/'+podName);
    },
    goToPodsMetricsTab: (namespace: string, podname: string) => {
      cy.visit(`/k8s/ns/${namespace}/pods/${podname}/metrics`)
    },
    goToPodsLogTab: (namespace:string, podname: string) => {
      cy.visit(`/k8s/ns/${namespace}/pods/${podname}/logs`)
    },
    checkContainerLastStateOnPodPage: (containerName, lastState) => {
      cy.get(`div.row div:contains("${containerName}")`).siblings(`div:contains('${lastState}')`).should('exist');
    },
    checkContainerLastStateOnContainerPage: (lastState) => {
      cy.get('dt:contains("Last State")').next('dd').should('contain',`${lastState}`);
    }
  }
  export const podsMetricsTab ={
    checkMetricsURL: (pos: number, chart: RegExp, chartdetails?: RegExp) =>{
      cy.get('[aria-label="View in query browser"]')
        .eq(pos)
        .should('have.attr','href')
        .and('match',chart)
        .and('match',chartdetails)
    },
    clickToMetricsPage: (pos: number,chart: RegExp) => {
      cy.get('[aria-label="View in query browser"]')
        .eq(pos)
        .should('have.attr','href')
        .and('match',chart)
        .then((href) => {
          cy.visit(href)
      })
    },
    checkMetricsLoaded: () => {
      let retries = 0;
      const checkMetrics = () => {
        /* cy.get() is an asynchronous operation,but I returned a
          boolean value before the loop completes, which leads to a
          mixture of asynchronous and synchronous code.Cypress will return error.
          So here I involed 'Promise'.
          it can be used to resolve or reject when asynchronous operations are completed,
          and they execute callbacks after the operation is completed.
          This ensures that a result is not returned until all asynchronous
          operations in the loop have completed */
        return new Promise((resolve, reject) => {
          let foundNoDatapoints = false;
          cy.get('[aria-label="View in query browser"]').each(($el) => {
            cy.wrap($el).should('not.contain','No datapoints found');
            foundNoDatapoints = true ;
          }).then(() => {
            if (foundNoDatapoints) {
              resolve(true);
            }else {
              resolve(false);
            }
          });
        });
      };
      const reloadAndWait = () => {
        cy.reload(true);
        cy.wait(50000); //wait for 5 seconds
      };
      const retry = () => {
        if (retries < 3) {
          cy.log(`Wait Metrics load Retry times ${retries +1} ...`);
          reloadAndWait();
          checkMetrics().then((continueRetry) => {
            retries++;
            if (continueRetry) {
              retry(); // if continueRetry = true, then retry
            }
          });
        }
      };
      checkMetrics().then((continueRetry) => {
        if(continueRetry) {
          retry;
        }
      });
    }
  }
