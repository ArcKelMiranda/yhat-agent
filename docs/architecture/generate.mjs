#!/usr/bin/env node
/**
 * generate.mjs — Deterministic Archify HTML generation for yhat-agent.
 *
 * Locates the repository root, validates required paths, invokes Archify
 * `deliver` with --quality standard, and writes the output to the fixed path.
 *
 * NOTE: `--quality standard` is used (not showcase) because showcase's
 * corridor-separation rules are incompatible with this compact 2-row layout
 * where multiple source nodes route to the same y-coordinate data layer.
 * Standard quality passes all checks with 0 errors.
 *
 * Requires: Node.js >= 18, Git submodule at tools/archify initialized.
 * Does not install packages or access remote resources.
 *
 * Usage:
 *   node docs/architecture/generate.mjs
 */

import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// --- Path resolution ---
const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(SCRIPT_DIR, '..', '..');

// --- Fixed paths ---
const ARCHIFY_CLI = path.resolve(REPO_ROOT, 'tools', 'archify', 'archify', 'bin', 'archify.mjs');
const INPUT_JSON = path.resolve(REPO_ROOT, 'docs', 'architecture', 'yhat-agent.architecture.json');
const OUTPUT_HTML = path.resolve(REPO_ROOT, 'docs', 'architecture', 'yhat-agent.architecture.html');

// --- Validation helpers ---
function requireFile(filePath, label) {
  if (!fs.existsSync(filePath)) {
    console.error(`Error: ${label} not found: ${filePath}`);
    process.exit(1);
  }
}

/**
 * Check that the Git submodule is initialized (not just a placeholder .git dir).
 * A bare `gitdir: ...` pointer file that points to a non-existent directory
 * indicates an uninitialized submodule.
 */
function requireSubmoduleInitialized(gitDirPath) {
  if (!fs.existsSync(gitDirPath)) {
    console.error(`Error: tools/archify submodule not initialized.`);
    console.error(`Run: git submodule update --init tools/archify`);
    process.exit(1);
  }
}

function runArchify(args, label) {
  const result = spawnSync(process.execPath, [ARCHIFY_CLI, ...args], {
    cwd: REPO_ROOT,
    encoding: 'utf-8',
    shell: false,
  });

  if (result.status !== 0) {
    console.error(`Error: Archify ${label} failed (exit ${result.status}).`);
    if (result.stderr) console.error(result.stderr);
    if (result.stdout) console.error(result.stdout);
    process.exit(1);
  }

  return result;
}

// --- Main ---
function main() {
  // 1. Validate repository root has a .git directory
  const gitDir = path.join(REPO_ROOT, '.git');
  if (!fs.existsSync(gitDir)) {
    console.error('Error: Repository root does not contain a .git directory.');
    process.exit(1);
  }

  // 2. Validate Archify CLI
  requireFile(ARCHIFY_CLI, 'Archify CLI');
  requireSubmoduleInitialized(path.join(REPO_ROOT, 'tools', 'archify', '.git'));

  // 3. Validate input JSON
  requireFile(INPUT_JSON, 'Architecture source JSON');

  // 4. Run Archify deliver
  // --quality standard: produces clean layout for this compact diagram
  // --repo-root: required for architecture type to resolve relative paths
  runArchify(
    [
      'deliver',
      'architecture',
      INPUT_JSON,
      OUTPUT_HTML,
      '--quality', 'standard',
      '--repo-root', REPO_ROOT,
    ],
    'deliver'
  );

  // 5. Verify output was written
  if (!fs.existsSync(OUTPUT_HTML)) {
    console.error(`Error: Archify did not produce expected output: ${OUTPUT_HTML}`);
    process.exit(1);
  }

  // 6. Report success
  const stats = fs.statSync(OUTPUT_HTML);
  console.log(`Archify HTML generated successfully.`);
  console.log(`  Source: ${path.relative(REPO_ROOT, INPUT_JSON)}`);
  console.log(`  Output: ${path.relative(REPO_ROOT, OUTPUT_HTML)}`);
  console.log(`  Size:   ${(stats.size / 1024).toFixed(1)} KB`);
}

main();
