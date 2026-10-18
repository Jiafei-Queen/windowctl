#!/usr/bin/env node
// Launcher shim for @muthuishere/windowctl.
// Resolves the installed platform sub-package and execs its native binary.

'use strict';

const { spawn } = require('child_process');

const SUPPORTED = {
  'darwin-arm64': '@muthuishere/windowctl-darwin-arm64',
  'darwin-x64': '@muthuishere/windowctl-darwin-x64',
  'linux-arm64': '@muthuishere/windowctl-linux-arm64',
  'linux-x64': '@muthuishere/windowctl-linux-x64',
  'win32-x64': '@muthuishere/windowctl-windows-x64',
};

const key = `${process.platform}-${process.arch}`;
const pkg = SUPPORTED[key];

function fail(msg) {
  process.stderr.write(`windowctl: ${msg}\n`);
  process.exit(1);
}

if (!pkg) {
  fail(
    `unsupported platform ${key}. Supported: ${Object.keys(SUPPORTED).join(', ')}. ` +
      `See https://github.com/Jiafei-Queen/windowctl/releases for manual downloads.`,
  );
}

const binName = process.platform === 'win32' ? 'windowctl.exe' : 'windowctl';

let binPath;
try {
  binPath = require.resolve(`${pkg}/bin/${binName}`);
} catch (_err) {
  fail(
    `platform package ${pkg} is not installed. This usually means npm was run with ` +
      `--no-optional or your environment skipped optional deps. Reinstall with: ` +
      `npm install -g @muthuishere/windowctl`,
  );
}

const child = spawn(binPath, process.argv.slice(2), { stdio: 'inherit' });
child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 1);
});
child.on('error', (err) => fail(`failed to launch ${binPath}: ${err.message}`));
