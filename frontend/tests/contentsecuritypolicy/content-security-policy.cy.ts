import { ClusterSettingPage } from '../../views/cluster-setting';

describe('Content Security Policy tests', () => {
  const [ login_idp, login_user, login_password, kubeconfig ] = [ Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'), Cypress.env('KUBECONFIG_PATH')];
  const query_console_dmeo_plugin_pod = `oc get deployment console-demo-plugin -n console-demo-plugin -o jsonpath='{.status.conditions[?(@.type=="Available")].status}'`;
  const query_console_customization_plugin_pod = `oc get deployment console-customization-plugin -n console-customization-plugin -o jsonpath='{.status.conditions[?(@.type=="Available")].status}'`;
  const query_console_configmap = `oc get cm console-config -n openshift-console -o jsonpath='{.data}' --kubeconfig ${kubeconfig}`;
  let checkCSPStatus = (plugin_name, csp_status) => {
    cy.get(`a[data-test="${plugin_name}"]`).parent().parent().parent('tr').within(() => {
      cy.get('td[data-label="csp-violations"]').should('include.text', csp_status);
    })
  };
  before(() => {
    cy.adminCLI('oc apply -f ./fixtures/contentsecuritypolicy/console-demo-plugin-manifests-csp.yaml');
    cy.adminCLI('oc apply -f ./fixtures/contentsecuritypolicy/console-customization-manifests-csp.yaml');
    cy.checkCommandResult(query_console_dmeo_plugin_pod, 'True', { retries: 4, interval: 15000 }).then(() => {
      cy.log('console-demo-plugin pod running successfully');
      return;
    });
    cy.checkCommandResult(query_console_customization_plugin_pod, 'True', { retries: 4, interval: 15000 }).then(() => {
      cy.log('console-customization pod running successfully');
      return;
    });
    cy.consoleBeforeUpdate();
    cy.adminCLI(`oc patch console.operator cluster --type='json' -p='[{"op": "add", "path": "/spec/plugins/-", "value":"console-demo-plugin"}]'`);
    cy.adminCLI(`oc patch console.operator cluster --type='json' -p='[{"op": "add", "path": "/spec/plugins/-", "value":"console-customization"}]'`);
    cy.wait(10000);
    cy.exec(`${query_console_configmap}`).its('stdout').should('match', /plugins.*console-customization.*console-demo-plugin/);
    cy.waitNewConsoleReady();
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${login_user}`);
  });
  after(() => {
    ClusterSettingPage.goToConsolePlugins();
    ClusterSettingPage.toggleConsolePlugin('console-customization', 'Disable');
    cy.adminCLI(`oc get console.operator cluster -o jsonpath='{.spec.plugins}'`).then((result) => {
      expect(result.stdout).not.include('"console-customization"')
    });
    ClusterSettingPage.toggleConsolePlugin('console-demo-plugin', 'Disable');
    cy.adminCLI(`oc get console.operator cluster -o jsonpath='{.spec.plugins}'`).then((result) => {
      expect(result.stdout).not.include('"console-demo-plugin"')
    });
    cy.adminCLI('oc delete -f ./fixtures/contentsecuritypolicy/console-demo-plugin-manifests-csp.yaml',{failOnNonZeroExit: false});
    cy.adminCLI('oc delete -f ./fixtures/contentsecuritypolicy/console-customization-manifests-csp.yaml',{failOnNonZeroExit: false});
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${login_user}`);
  });

  // Content Security Policy basic functionality
  it('(OCP-77556,yapei,UserInterface)Notify user of console plugin related CSP violations',{tags:['@userinterface','@e2e','@techpreview','admin','@hypershift-hosted']}, () => {
    cy.uiLogin(login_idp, login_user, login_password);
    // content-security-policy-report-only header exist
    cy.request('/').then((response) => {
      const csp_report_only = response.headers['content-security-policy-report-only'];
      expect(csp_report_only).to.include("script-src 'self' 'unsafe-eval' 'nonce-");
    });
    cy.wait(5000);
    cy.visit('/dynamic-route-1', {
      onBeforeLoad (win) {
        cy.spy(win.console, 'warn').as('console.warn');
      }
    });
    // show CSP violation in browser console log
    cy.wait(10000);
    cy.get('@console.warn').should('be.calledWith', "Content Security Policy violation seems to originate from plugin console-demo-plugin");

    // show CSP violation status in Console plugins tab
    // due to OCPBUGS-44991,csp status is always No
    ClusterSettingPage.goToConsolePlugins();
    checkCSPStatus('console-demo-plugin', 'No');
  });

  it('(OCP-77059,yapei,UserInterface)Console-operator should configure console with the CSP allowed directives)',{tags:['@userinterface','@techpreview','admin','@hypershift-hosted']}, function() {
    cy.isTechPreviewNoUpgradeEnabled().then(value => {
      if (value === false) {
        cy.log('Skip the case because TP not enabled!!');
        this.skip();
      }
    });
    const patch_console_customization = `oc patch consoleplugin console-customization -p '{"spec":{"contentSecurityPolicy":[{"directive": "ScriptSrc","values":["https://script1.com","https://script2.com"]},{"directive":"StyleSrc","values":["http://style1.com","http://style2.com"]}]}}' --type merge`;
    const patch_console_demo_plugin = `oc patch consoleplugin console-demo-plugin -p '{"spec":{"contentSecurityPolicy":[{"directive": "ScriptSrc","values":["https://script1.com","https://script3.com"]},{"directive":"ImgSrc","values":["https://imagesource1.com"]},{"directive":"DefaultSrc","values":["https://defaultsource1.com","https://defaultsource2.com","catfact.ninja"]}]}}' --type merge`;
    cy.consoleBeforeUpdate();
    cy.adminCLI(`${patch_console_customization}`).its('stdout').should('include', 'patched');
    cy.adminCLI(`${patch_console_demo_plugin}`).its('stdout').should('include', 'patched');
    cy.wait(10000);
    cy.exec(`${query_console_configmap}`).its('stdout').should('match', /contentSecurityPolicy.*DefaultSrc.*catfact\.ninja.*defaultsource1.com.*defaultsource2.com.*ImgSrc.*imagesource1.com.*ScriptSrc.*script1.com.*script2.com.*script3.com.*StyleSrc.*style1.com.*style2.com/);
    //wait for new console pods started successfully
    cy.waitNewConsoleReady();
    cy.uiLogout();
    cy.uiLogin(login_idp, login_user, login_password);
    cy.request('/').then((response) => {
      const csp_report_only = response.headers['content-security-policy-report-only'];
      expect(csp_report_only).to.include("default-src 'self' catfact.ninja https://defaultsource1.com https://defaultsource2.com");
    });
    cy.visit('/dynamic-route-1', {
      onBeforeLoad (win) {
        cy.spy(win.console, 'warn').as('console.warn');
      }
    });
    // show CSP violation in browser console log
    cy.wait(10000);
    cy.get('@console.warn').should('not.always.have.been.calledWith', "Content Security Policy violation seems to originate from plugin console-demo-plugin");
  });
});