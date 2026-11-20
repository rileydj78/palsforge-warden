# Code signing policy

Windows builds of Palsforge Warden are **not signed yet**. SignPath Foundation's free programme
for open-source projects asked us to reapply once the project is better established (November
2026). Until a signing route is in place, each release lists the SHA-256 checksums of its files,
built by this repository's GitHub Actions workflow. The rules below apply as soon as signing starts.

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
