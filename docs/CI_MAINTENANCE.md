# CI action and shell-check maintenance

The release workflow pins third-party actions to full upstream commit SHAs and
keeps the corresponding release tag beside each pin. To update a pin, select a
specific upstream release, inspect its release notes and action metadata, resolve
the tag through the upstream Git reference API, and confirm that the returned
commit SHA is the immutable commit for that release. Update both the SHA and tag
comment together, then run `actionlint -shellcheck=''`, the release workflow's
targeted tests, and hosted ARM release CI. The separate workflow ShellCheck step
keeps the initial file scope explicit. Do not copy a floating major tag or a
short SHA.

Current upstream references:

- [`actions/checkout` v6.0.3](https://github.com/actions/checkout/releases/tag/v6.0.3), commit [`df4cb1c069e1874edd31b4311f1884172cec0e10`](https://github.com/actions/checkout/commit/df4cb1c069e1874edd31b4311f1884172cec0e10); Node 24 runtime, runner v2.327.1 or newer.
- [`actions/setup-go` v6.0.0](https://github.com/actions/setup-go/releases/tag/v6.0.0), commit [`44694675825211faa026b3c33043df3e48a5fa00`](https://github.com/actions/setup-go/commit/44694675825211faa026b3c33043df3e48a5fa00); Node 24 runtime, runner v2.327.1 or newer.
- [`softprops/action-gh-release` v3.0.0](https://github.com/softprops/action-gh-release/releases/tag/v3.0.0), commit [`b4309332981a82ec1c5618f44dd2e27cc8bfbfda`](https://github.com/softprops/action-gh-release/commit/b4309332981a82ec1c5618f44dd2e27cc8bfbfda)

`setup-go` caching is disabled because this repository has no external Go
module dependencies or `go.sum`; do not add a checksum file just to enable a
cache lookup. If external modules are added later, restore caching with an
explicit dependency file path and commit the resulting `go.sum` as normal.

The release workflow applies ShellCheck at warning severity to the installer and
startup script first. It keeps `sh -n` over all shipped shell scripts while the
remaining legacy firmware commands and BusyBox applet constraints are reviewed
in smaller steps. Expand ShellCheck file coverage as each script's intentional
word splitting and target-firmware command support are verified; do not blanket
disable rules or assume the host shell's applets match firmware.
