# windowctl (npm)

This is the npm package for [windowctl](https://github.com/Jiafei-Queen/windowctl),
a cross-platform CLI for managing desktop windows and monitors.

## Install

```sh
npx windowctl windows list
```

```sh
npm install -g windowctl
```

The package's `postinstall` step downloads the native binary for your
OS / arch from the matching GitHub release and unpacks it next to the
JS wrapper. If the install step fails, see
<https://github.com/Jiafei-Queen/windowctl/releases> for available
versions.

## How it works

- `bin/windowctl.js` — JS wrapper that locates and exec's the native
  binary, forwarding args, stdio, and exit code.
- `scripts/install.js` — postinstall that detects `process.platform` /
  `process.arch`, downloads `windowctl_<version>_<os>_<arch>.tar.gz`
  from the matching GitHub release, and extracts it into `./native/`.

The package itself is platform-agnostic; only the downloaded binary is
platform-specific.
