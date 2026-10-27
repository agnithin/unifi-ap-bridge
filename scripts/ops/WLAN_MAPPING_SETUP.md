# On-demand Mac WLAN mapping

Run `python3 -B scripts/ops/mac_wlan_mapping.py` from the trusted Mac after a
UniFi WLAN is renamed or replaced. It streams the bounded resolver source to
CT107 using the existing strict-known-host PVE SSH route. The resolver reads
`/etc/ops-broker/unifi-api-key` inside CT107 and returns only the exact
`blueberry` and `blueberry-prime` names with validated legacy 24-hex ObjectIds.
The Mac then uses its existing strict-known-host SSH access to publish that
small mapping to Main and Node. No API key is read or stored on the Mac or APs.

The defaults are PVE `root@10.100.1.100`, controller `https://10.100.1.1`,
site `default`, Main `nithin@10.100.1.10`, and Node `nithin@10.100.1.11`. The
Mac's existing SSH identity and `known_hosts` must already authorize those
routes; the command uses BatchMode and refuses unknown host keys. It creates no
keys and changes no trust. Override a host with repeated `--ap-host` options.

Controller TLS uses CT107's system trust by default. If that controller
certificate needs a private CA bundle already on CT107, pass its CT path with
`--ca-file`. Use `--insecure` only if the operator explicitly accepts disabling
controller certificate verification on the private LAN.

The AP cache is `/jffs/addons/unifi-bridge/config/wlan-mapping.json`, mode
`0600`, atomically replaced only when the mapping changes. The publisher checks
the remote SHA-256 and mode before and after a write. First run the command to publish and verify both caches. Then
activate this path in the AP bridge configuration as `wlan_mapping_file`. The
current manual `wlan_mapping` entries take precedence, so remove only the two
target SSID keys in the same reviewed settings change; retain unrelated entries.
Changing `wlan_mapping_file` is a one-time settings operation that may restart
the bridge and sync its Node. Once configured, later cache-content updates are
read without restart, and invalid refreshes preserve the last valid mapping.

This is an on-demand Mac command; it installs no timer and does not modify AP
configuration. Each run performs one bounded controller GET and one atomic
cache update per AP only if needed. If one AP publish fails, the other may
already have the same valid mapping; rerunning is safe and skips identical
content.

## Rollback

Disable automatic file use by removing `wlan_mapping_file` from the reviewed AP
configuration. Restore only the two manual target entries from the private
configuration backup if their previous behavior is wanted. Remove the cache
file over the existing Mac SSH route if cleanup is needed:

```sh
ssh -o BatchMode=yes -o StrictHostKeyChecking=yes nithin@10.100.1.10 \
  'rm -f /jffs/addons/unifi-bridge/config/wlan-mapping.json /jffs/addons/unifi-bridge/config/wlan-mapping.json.tmp.*'
ssh -o BatchMode=yes -o StrictHostKeyChecking=yes nithin@10.100.1.11 \
  'rm -f /jffs/addons/unifi-bridge/config/wlan-mapping.json /jffs/addons/unifi-bridge/config/wlan-mapping.json.tmp.*'
```

These are operator-run rollback steps only. Source validation does not contact
CT107 or write to either AP.
