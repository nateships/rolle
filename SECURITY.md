# Security policy

## Report a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/nateships/rolle/security/advisories/new). It opens a private thread with the maintainer. Do not open a public issue for a security problem.

Include the version (`rolle --version` or **Settings → About**), the platform, and steps that show the problem. A support bundle (`rolle support`) helps; it redacts account ids, emails, hosts, and secrets.

You get a first reply within seven days. A confirmed problem gets a fix in a release and a GitHub security advisory that credits you, unless you ask otherwise.

## Supported versions

The latest release on the [releases page](https://github.com/nateships/rolle/releases) gets fixes. The desktop app updates itself; the CLI comes from the same release.

## Scope

In scope: the desktop app, the `rolle` command, the update channel, and the release pipeline in this repository. Out of scope: the cloud providers' own services and the third-party tools rolle drives (`aws`, `az`, `gcloud`).

## What the project does today

See [Security model](https://getrolle.com/docs/security): secrets stay in the OS keychain, releases are signed and carry build provenance, and every network destination is listed and checkable.
