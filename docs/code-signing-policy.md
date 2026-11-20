# Code signing policy

Free code signing for the Windows release of Palsforge Warden is provided by
[SignPath.io](https://signpath.io), certificate by [SignPath Foundation](https://signpath.org).
(Pending: the project has applied; until approval, Windows builds are unsigned.)

## What is signed

Only `warden.exe` built by this repository's GitHub Actions workflow (`.github/workflows/build.yml`)
from a tagged commit on `main`. Nothing built on a personal machine is ever signed.

## Team roles

- **Committers and reviewers:** [rileydj78](https://github.com/rileydj78)
- **Approvers** (who approve each signing request): [rileydj78](https://github.com/rileydj78)

Every change reaches `main` through this repository. Code from outside contributors is reviewed
before it is merged.

## Privacy

This program will not transfer any information to other networked systems unless specifically
requested by the user or the person installing or operating it.

In practice: Warden talks to your own Palworld server on the same machine (its REST API), and,
only if you set them up, to the backup places and notification services you choose. It has no
telemetry, no update check and no account with us.
