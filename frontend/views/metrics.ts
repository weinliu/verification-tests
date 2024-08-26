export const metricsTab = {
  checkMetricsLoadedWithoutError: () => {
    const maxRetries = 10;
    const pageLoadingDelay = 20000; // 20 seconds
    const retryDelay = 30000; //30 seconds
    const textToCheck = 'No datapoints found';
    const checkMetricsContent = ($metrics, retryCount = 0) => {
      const checkNoDataPointsFound = $metrics.toArray().every(el => {
        const metricText = Cypress.$(el).text().trim();
        const containsText = !metricText.includes(textToCheck);
        if (metricText === '') {
            return false;
        }
        cy.log(`METRIC TEXT: ${metricText} include/uninclude "${textToCheck}": return ${containsText}`);
        return containsText;
      });

      if (!checkNoDataPointsFound) {
        if (retryCount === maxRetries) {
          cy.log("Max retries exceeded, not all metrics/metric data loaded as expected");
          throw new Error('Max retries exceeded, not all metrics/metric data loaded as expected');
        }

        cy.wait(retryDelay).then(() => {
          cy.log("Trigger page reload");
          cy.reload();
          cy.wait(pageLoadingDelay) // wait for page loading success
          cy.get('.query-browser__table').then(($els) => {
            checkMetricsContent($els, retryCount + 1);
          });
        });
      } else {
        cy.log('All metrics Loaded. And chart do not contain "No datapoints found"');
      }
    };
    cy.wait(pageLoadingDelay);
    cy.get('.query-browser__table').then(($els) => {
      if ($els.text().includes(textToCheck)) {
        checkMetricsContent($els);
      } else {
        cy.log('All metrics Loaded. And chart do not contain "No datapoints found"');
      }
    });
  }
}
