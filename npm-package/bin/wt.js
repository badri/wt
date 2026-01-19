#!/usr/bin/env node

const { spawn } = require('child_process');
const path = require('path');
const os = require('os');
const fs = require('fs');

// Determine the platform-specific binary name and path
function getBinaryPath() {
  const platform = os.platform();
  const arch = os.arch();

  let binaryName = 'wt';
  if (platform === 'win32') {
    binaryName = 'wt.exe';
  }

  // Binary is stored in the package's bin directory
  const binaryPath = path.join(__dirname, binaryName);

  if (!fs.existsSync(binaryPath)) {
    console.error(`Error: wt binary not found at ${binaryPath}`);
    console.error('This may indicate that the postinstall script failed to download the binary.');
    console.error(`Platform: ${platform}, Architecture: ${arch}`);
    console.error('');
    console.error('Try reinstalling: npm install -g @worktree/wt');
    console.error('Or install manually: go install github.com/badri/wt/cmd/wt@latest');
    process.exit(1);
  }

  return binaryPath;
}

// Execute the native binary with all arguments passed through
function main() {
  const binaryPath = getBinaryPath();

  // Spawn the native wt binary with all command-line arguments
  const child = spawn(binaryPath, process.argv.slice(2), {
    stdio: 'inherit',
    env: process.env
  });

  child.on('error', (err) => {
    console.error(`Error executing wt binary: ${err.message}`);
    process.exit(1);
  });

  child.on('exit', (code, signal) => {
    if (signal) {
      process.exit(1);
    }
    process.exit(code || 0);
  });
}

main();
