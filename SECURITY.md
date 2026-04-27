# Security Policy

Harness Yard is currently pre-1.0. Thank you for helping keep the project and its users secure.

## Supported Versions

Security fixes are applied to the following versions:

| Version / branch | Supported |
| --- | --- |
| `main` | Yes |
| Latest published release | Yes, once release artifacts are available |
| Older releases, tags, or prereleases | No |
| Forks or downstream modifications | No |

Because Harness Yard is pre-1.0, security fixes may be released as a new version rather than backported. If no release artifacts exist for the affected code, the fix will be applied to `main`.

## Reporting a Vulnerability

Please do **not** report security issues in public GitHub issues, pull requests, or discussions.

Use GitHub's private vulnerability reporting flow:

<https://github.com/zack-nova/harnessyard/security/advisories/new>

When reporting a vulnerability, please include as much of the following as possible:

- affected version, tag, or commit SHA
- operating system and architecture
- clear reproduction steps or proof of concept
- expected impact and affected components
- whether the issue is known to be actively exploited
- any known workaround or mitigation
- whether you would like public credit after disclosure

If the GitHub private reporting flow is unavailable, please open a public issue asking for a private security contact, but do not include vulnerability details in the public issue.

## Scope

Examples of in-scope reports include:

- vulnerabilities in Harness Yard source code
- vulnerabilities in release artifacts published by this project
- issues that affect confidentiality, integrity, availability, authentication, authorization, or supply-chain integrity
- security-relevant problems in project-maintained automation or packaging

Examples of out-of-scope reports include:

- vulnerabilities only present in unsupported forks or downstream modifications
- reports without a plausible security impact
- denial-of-service testing against systems you do not own or control
- social engineering, phishing, or physical attacks
- dependency vulnerabilities that do not create an exploitable condition in Harness Yard

## Handling Process

We aim to acknowledge security reports within 7 days.

After acknowledgment, we will:

1. validate the report and assess severity;
2. coordinate with the reporter through the advisory thread;
3. prepare and test a fix privately when appropriate;
4. release a fix, patch, or mitigation;
5. publish a security advisory when disclosure is appropriate.

For confirmed issues, we aim to provide status updates at least every 14 days while the issue is being investigated or fixed.

## Coordinated Disclosure

Please keep vulnerability details private until a fix or mitigation is available and an advisory has been published, unless otherwise coordinated with the maintainers.

We will work with reporters on a reasonable disclosure timeline. If a fix requires more time, we will coordinate next steps, mitigations, and disclosure timing through the private advisory thread.

## Rewards and Credit

Harness Yard does not currently offer a bug bounty program.

We are happy to credit reporters in the advisory or release notes when they want credit and disclosure is appropriate.