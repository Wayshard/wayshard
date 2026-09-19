# Release integrity public key

Commit the minisign **public** key here as `wayshard-release.minisign.pub`
after generating it with `scripts/release/generate-signing-material.sh`
(or the exact `minisign -G` command in `docs/release.md`).

Users verify a release with:

```sh
minisign -V -p keys/wayshard-release.minisign.pub -m SHA256SUMS.txt
sha256sum -c SHA256SUMS.txt
```

Never commit `*.sec`, `*.pfx`, `*.jks`, `*.p12`, or `*.key`. This directory's
`.gitignore` allows only `README.md` and `*.pub`.

The public key proves the checksum manifest was signed by the Wayshard
maintainer. It does not make Apple or Microsoft trust the binaries.
