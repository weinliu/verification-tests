import { dashboard } from "views/dashboards-page"
describe('Monitoring dashboards related features', () => {
    before(() => {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    });
    after(() => {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
        cy.logout;
    });
    it('(OCP-59699,juzhao,Cluster_Observability) Add node-role dropdown to node related dashboards', { tags: ['e2e', 'admin', '@smoke'] }, () => {
        // 4.16 "Kubernetes / Compute Resources / Node (Pods)" dashboard name is dashboard-k8s-resources-node, 4.15 is grafana-dashboard-k8s-resources-node
        dashboard.visitDashboard('dashboard-k8s-resources-node');
	cy.byTestID('role-dropdown').should('exist').click();
	cy.contains('button', 'master').should('exist').click();
	cy.byTestID('node-dropdown').should('exist').click();
	cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} get node -l node-role.kubernetes.io/master --no-headers | sort | awk '{print $1}'`, { failOnNonZeroExit: false }).then((output) => {
    	    if (output.stdout.trim() !== '') {
    	        const nodes = output.stdout.trim().split('\n');
	        cy.log(`master nodes: ${nodes}`);

    	        nodes.forEach((node) => {
        	    cy.get('.pf-v5-c-select__menu').find('button').contains(new RegExp(node)).should('exist');
    	        });
	    }
	});

        // 4.16 "Node Exporter / USE Method / Node" dashboard name is dashboard-node-rsrc-use, 4.15 is grafana-dashboard-node-rsrc-use
        dashboard.visitDashboard('dashboard-node-rsrc-use');
	cy.byTestID('role-dropdown').should('exist').click();
	cy.contains('button', 'worker').should('exist').click();
	cy.byTestID('instance-dropdown').should('exist').click();
	cy.exec(`oc --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} get node -l node-role.kubernetes.io/worker --no-headers | sort | awk '{print $1}'`, { failOnNonZeroExit: false }).then((output) => {
    	    if (output.stdout.trim() !== '') {
    	        const nodes = output.stdout.trim().split('\n');
	        cy.log(`worker nodes: ${nodes}`);

    	        nodes.forEach((node) => {
        	    cy.get('.pf-v5-c-select__menu').find('button').contains(new RegExp(node)).should('exist');
    	        });
	    }
	});
    });

});
