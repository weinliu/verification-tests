# Contributing

## Test case naming rules
Please refer to [this guide](https://docs.google.com/document/d/1k761p65J0Ig81hwZWaw73QJV6SBa7kPxbjuTEBlBbF4/edit#heading=h.shrqeb9dx5rz)
for general naming instructions.

TODO: add intro for Hive-specific tags

## How to open a PR

1. Pull the latest code:
   ```shell
   git checkout master
   git pull $PUBLIC_REPO master
   ```
2. Create a new branch and switch to it: 
   ```shell
   git checkout -b $BRANCH_NAME
   ```
3. Make changes to the code. 
4. Run static code checks, fix any errors, repeat until no errors persist:
   ```shell
   go mod tidy
   ``` 
5. Commit and push the changes to your forked repository.
6. Rehearse the test cases affected by your change, fix any failures:
   - Option 1 (local rehearsal): 
   ```shell
   make all
   ./bin/extended-platform-tests run all --dry-run | \
   grep -E "(12345|23456)" | \
   ./bin/extended-platform-tests run --timeout 60m --include-success -o OCP-12345-23456.txt -f -
   ```
   - Option 2 (Jenkins rehearsal, recommended): refer to [docs](https://github.com/openshift/openshift-tests-private#jenkins) for instructions
7. Open a PR on GitHub ([example](https://github.com/openshift/openshift-tests-private/pull/10706))

## Code review
During the code review process, the following labels are required before a PR can be merged automatically:
1. An `/lgtm` (abbreviation for "looks good to me") label given by a team member
2. An `/approve` label given by a maintainer

Additionally, it is important to approach code reviews with an open mind and maintain a positive 
and constructive attitude.

# Test case rehearsals

VSphere test cases are meant to be rehearsed locally (instead of on Jenkins). 
This is because an additional set of AWS credentials are required for DNS setup. 
These credentials are only available locally, and will be loaded by AWS tool chains.
In addition, a stable VPN connection is required for running VSphere test cases.

In addition, vSphere test cases can only be executed on ci-vlan clusters and not on the DevQE ones.
This is because IP address reservation for DevQE is managed through IPAM, which requires
a token that is exclusively accessible in aosqe/cucushift-internal (a private repository), 
making it complex to obtain programmatically. 

To acquire a ci-vlan cluster, there are a few options available:
- Use the `launch` or `workflow-launch` command with the clusterbot Slack app. 
Clusters obtained this way are only accessible for approximately 2 hours and 30 minutes.

# Supported platforms

## AWS
Test cases for AWS and AWS Government are located in hive_aws.go. 

## Azure
Test cases for MAC and MAG are located in hive_azure.go.

## GCP
Test cases for GCP are located in hive_gcp.go. 

## VSphere
Test cases for vSphere are located in hive_vsphere.go.

## General platform
Test cases which support multiple platforms should be placed in hive.go.

# Resource management

## API calls
The frequency of API calls (to the API server or cloud provider endpoints) should be limited, 
especially in the following scenarios:
- In functions to poll
- In (in)finite for loops

Moreover, it is possible to store test case independent information (e.g. platform-specific data) 
in the ephemeral cluster (the Hive cluster in our case).
This approach eliminates the need for making repetitive GET requests prior to executing each test case.

## Cloud resources
To avoid excessive cloud resource consumption, we should:
- Avoid the co-existence of more than two (non-fake) ClusterDeployments.
- Consider the usage of fake clusters whenever possible.

## Parallelism
The design of the platform provides parallelism on the test case level: each test case runs in its own process.
On the other hand, it is generally not recommended to manually spawn additional Go routines. 

# Dependency management

## Bumps
In case of updates to go.mod, please run at least one test case (Hive or not) 
which makes use of each new package version.

## openshift/installer

Please avoid requiring openshift/installer types whenever possible as they bring in quite a few dependencies, 
making this repository unnecessarily difficult to maintain. 

For a minimal install-config, use the `minimalInstallConfig` type, and extend it if necessary.

# Miscellaneous

## Timeouts
The majority of test cases for Hive involve cluster installation, so special care must be taken
to avoid timeouts. 
Please note that the timeout is 100min per test case for Azure and 90min per test case
for other platforms.

## Backports
Bug fixes are sometimes cherry-picked to earlier branches. If you're unsure about something, consult with the team.

## Import style
Please follow [this guide](https://github.com/uber-go/guide/blob/master/style.md#import-group-ordering) for import grouping.

## TODOs
If a TODO is only for you, please enclose your GitHub ID within parentheses, e.g. TODO(my-github-id).

## Code quality
Readability, reusability, maintainability, reliability, scalability and performance are all 
important factors, especially for public utilities living in openshift-tests-private/test/extended/util/.

## Cloud account agnosticism
Multi-account support is achieved by dynamically acquiring platform configurations during runtime.
