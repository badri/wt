#!/usr/bin/env node

const { execSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

// Test that the binary exists and runs
function test() {
  const binDir = path.join(__dirname, '..', 'bin');
  const binaryName = os.platform() === 'win32' ? 'wt.exe' : 'wt';
  const binaryPath = path.join(binDir, binaryName);

  console.log('Testing wt installation...');
  console.log(`Binary path: ${binaryPath}`);

  // Check binary exists
  if (!fs.existsSync(binaryPath)) {
    console.error('FAIL: Binary not found');
    process.exit(1);
  }
  console.log('PASS: Binary exists');

  // Check binary is executable
  try {
    const output = execSync(`"${binaryPath}" version`, { encoding: 'utf8' });
    console.log(`PASS: Binary runs - ${output.trim()}`);
  } catch (err) {
    console.error(`FAIL: Binary failed to execute: ${err.message}`);
    process.exit(1);
  }

  console.log('All tests passed!');
}

test();
