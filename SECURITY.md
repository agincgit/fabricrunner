# Security Policy

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub's private
vulnerability reporting for this repository. Include affected versions,
reproduction steps, impact, and any suggested mitigation.

Do not include real credentials, non-public source code, customer data, or other
sensitive material in a report.

## Security posture

Fabric Runner treats model output, tool descriptions, provider responses, and
remote workers as untrusted. Side-effecting tools are denied when a required
sandbox cannot be established. Cross-zone data movement is denied when policy
cannot make a definitive decision.

The project is pre-alpha. No release should be treated as a security boundary
until the relevant behavior has an explicit acceptance test and supported
platform declaration.
