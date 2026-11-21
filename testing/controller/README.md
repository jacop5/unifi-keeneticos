# Local UniFi OS test controller

The active deployment is **UniFi OS Server 5.1.42**, installed from Ubiquiti's
official Linux ARM64 installer in the OrbStack machine `unifi-os-test`
(Ubuntu 24.04, 2 CPU cores, 4 GiB RAM limit, 32 GiB disk limit).
It runs its own Podman/systemd services; it is not the standalone Network
Application Docker image.

Web UI: https://localhost:11443 or https://unifi-os-test.orb.local:11443.
Installation and HTTP 200 responses were verified. Initial console setup
was completed by the user. The certificate is self-signed.

Recreate the dedicated environment with `sh testing/controller/install-uos.sh`.
This downloads the pinned official ARM64 installer and installs the required
Podman/systemd services in the named VM. Stop the archived Docker stack first
if it was manually restarted. Console credentials are not stored in this repo.

OrbStack forwards the machine's ports to the Mac. The inform listener was
verified at `http://192.168.1.76:8080/inform` (HTTP 400 for a bare GET, as
expected for the binary POST endpoint). Recheck the Mac's Wi-Fi address if
DHCP changes it. Both legacy Docker services were verified stopped.

Service status: `orb -m unifi-os-test -u root systemctl status uosserver`

Stop/start: `orb -m unifi-os-test -u root systemctl stop uosserver`
and `orb -m unifi-os-test -u root systemctl start uosserver`.

The legacy Docker stack below has been stopped. Its volumes and files are
retained for recovery; do not start both controllers during testing.

## Archived legacy deployment

Dedicated OrbStack Compose project: `unifi-keenetic-test`.
UniFi Network 10.6.109 (LinuxServer build ls155), with MongoDB 7.0.
Images are pinned by digest. Database and controller state use dedicated
Docker volumes; database credentials are in the git-ignored `.env` (mode 600).
MongoDB has no published host port.

Start: `docker compose -f testing/controller/compose.yaml up -d`

Stop while retaining data: `docker compose -f testing/controller/compose.yaml stop`

Web setup: https://localhost:8443 (self-signed certificate).
The first-run wizard has not yet been completed; no controller admin account
has been created.

At creation the Mac's Wi-Fi address was `192.168.1.76`, so the router-reachable
inform endpoint is `http://192.168.1.76:8080/inform`. Confirm the address after
network changes. Set the controller's Inform Host Override to that reachable
address before adopting; the internal Docker address is unsuitable.

## Router test constraints

The Mac is connected through the Hero DSL's serving Wi-Fi. Do not change
the serving SSID, credentials, bridge membership, channel or shared radio
settings during a test. Use a separately named SSID on a verified unused AP
slot. A separate SSID alone does not isolate radio-wide changes: the current
planner can change radio settings, so its full `-apply` mode is not suitable
for this connected test until radio changes are explicitly excluded.

No router configuration was modified when creating this controller.
