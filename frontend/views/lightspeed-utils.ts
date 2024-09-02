import { listPage } from "../upstream/views/list-page";
import { operatorHubPage } from "./operator-hub-page";

//If specific channel/catsrc needed for testing, export the values using CYPRESS_EXTRA_PARAM before running the lightspeed tests
//ex: export CYPRESS_EXTRA_PARAM='{"openshift-lightspeed": {"cluster-lightspeed": {"channel": "preview", "version" : "0.1.0", "source": "redhat-operators"}}}'
const extraParam = Cypress.env("EXTRA_PARAM");

export const catalogSource = {
  //set channel
  channel: (packageName) => {
    return extraParam?.['openshift-lightspeed']?.[packageName]?.channel ?? "preview";
  },
  //set version (availabe for OCP >= 4.15)
  version: (packageName) => {
    return extraParam?.['openshift-lightspeed']?.[packageName]?.version ?? "0.1.0";
  },  
};

export const lightUtils = {
  // TODO: Create utils for lightspeed Operator
};
