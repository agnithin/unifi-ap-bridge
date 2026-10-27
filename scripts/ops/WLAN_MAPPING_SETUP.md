# Host WLAN mapping refresh

The bridge reads `/jffs/addons/unifi-bridge/config/wlan-mapping.json` when
`wlan_mapping_file` is configured. `refresh_wlan_mapping.py` resolves only the
exact `blueberry` and `blueberry-prime` SSIDs with the existing read-only
classic WLAN-list GET, then sends only those names and validated 24-hex legacy
ObjectIds to each configured AP over SSH. The API key stays at
`/etc/ops-broker/unifi-api-key` on CT107. The AP file is mode `0600` and is
atomically replaced on persistent storage only when the content changes; the
host compares remote SHA-256 before writing. A failed controller read does not
publish anything, and the mapping survives AP reboot without placing API
credentials on the device.

## Prepare on CT107

Install the matching bridge source tree under `/opt/unifi-ap-bridge` and copy
these files into its `scripts/ops/` directory:

```text
wlan_mapping.py
refresh_wlan_mapping.py
unifi-wlan-mapping-refresh.service
unifi-wlan-mapping-refresh.timer
```

Create `/etc/ops-broker/unifi-wlan-mapping.json` as root-owned mode `0600`:

```json
{
  "controller": "https://<controller-address>",
  "site": "default",
  "identity_file": "/etc/ops-broker/unifi-ap-mapping-ed25519",
  "ap_hosts": ["nithin@10.100.1.10", "nithin@10.100.1.11"],
  "ca_file": "/etc/ssl/certs/unifi-controller-ca.pem"
}
```

Read-only verification found CT107's existing key is not authorized on either
AP. Strict host-key checks passed through to user authentication on both APs,
but each returned `Permission denied (publickey,password)`. Do not enable the
timer until the operator approves a dedicated CT107 key and authorizes its
public half for the existing AP admin account. This service requires shell
access as that account to write the fixed mapping file. Never copy an AP cluster
private key to CT107. The script refuses unknown SSH host keys.
The dedicated identity file path above must be root-owned mode `0600`; the
publisher explicitly selects it and will not fall back to CT107's generic key.
The controller TLS connection must use a trusted CA file, or explicitly set
`"insecure": true` for a local self-signed controller;
the latter encrypts transport but does not authenticate the controller and is
not the preferred setup. The existing API key file must be regular, contain a
single token, and have no group/other permission bits.

After the first successful publish has been verified on both APs, change the AP
configuration to set `wlan_mapping_file` to
`/jffs/addons/unifi-bridge/config/wlan-mapping.json` and remove only the
`blueberry` and `blueberry-prime` keys from its manual `wlan_mapping` object.
Manual entries take precedence over the refreshed file; leaving either targeted
entry means that SSID continues using its manual ID. Preserve unrelated manual
entries. A refresh does not restart the bridge.

Run the publisher once before the AP configuration change; after verifying the
file on both APs and applying the AP setting above, enable the timer:

```sh
python3 -B /opt/unifi-ap-bridge/scripts/ops/refresh_wlan_mapping.py
install -m 0644 /opt/unifi-ap-bridge/scripts/ops/unifi-wlan-mapping-refresh.service /etc/systemd/system/
install -m 0644 /opt/unifi-ap-bridge/scripts/ops/unifi-wlan-mapping-refresh.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now unifi-wlan-mapping-refresh.timer
```

The timer refreshes every five minutes and one minute after CT boot. AP flash is
written only when either resolved ObjectId changes. Inspect
only sanitized state with `systemctl status` and `journalctl -u
unifi-wlan-mapping-refresh.service`; success logs contain no mapping values or
credentials. The last valid mapping remains on AP flash if the controller is
unavailable, so current mappings remain available across reboots. A deleted or
renamed controller WLAN cannot be detected while the controller is offline; once
it returns, the exact-name resolver must succeed before replacing the cache.

## Rollback

Disable scheduling and remove the AP mapping file, then remove the bridge's
`wlan_mapping_file` setting. Preserve any unrelated manual mappings. The
manual entries for the two target SSIDs may be restored from the private AP
configuration backup if rollback requires their prior behavior.

```sh
systemctl disable --now unifi-wlan-mapping-refresh.timer
rm -f /etc/systemd/system/unifi-wlan-mapping-refresh.{service,timer}
systemctl daemon-reload
ssh root@<main-ssh-host> 'rm -f /jffs/addons/unifi-bridge/config/wlan-mapping.json /jffs/addons/unifi-bridge/config/wlan-mapping.json.tmp.*'
ssh root@<node-ssh-host> 'rm -f /jffs/addons/unifi-bridge/config/wlan-mapping.json /jffs/addons/unifi-bridge/config/wlan-mapping.json.tmp.*'
rm -f /etc/ops-broker/unifi-wlan-mapping.json
```

These are prepared operator steps only; source validation does not run them or
write to CT107/APs. Rollback must also remove the dedicated CT107 public key
from both AP admin accounts and delete `/etc/ops-broker/unifi-ap-mapping-ed25519`
from CT107.
