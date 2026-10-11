// Runs every test suite in the repository and stops at the first failure.
// Used by `npm test` at the repository root (and by the stop hook in .claude/hooks).
import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const suites = [
  { name: "packages/validation (Go)", cwd: "packages/validation", cmd: "go", args: ["test", "./..."] },
  { name: "services/agent (Go)", cwd: "services/agent", cmd: "go", args: ["test", "./..."] },
  { name: "apps/api (Go)", cwd: "apps/api", cmd: "go", args: ["test", "./..."] },
  { name: "apps/panel (UI)", cwd: "apps/panel", cmd: "npm", args: ["test", "--silent"] },
];

for (const suite of suites) {
  console.log(`\n== ${suite.name}`);
  const result = spawnSync(suite.cmd, suite.args, {
    cwd: path.join(root, suite.cwd),
    stdio: "inherit",
    shell: process.platform === "win32",
    env: process.env,
  });
  if (result.error) {
    console.error(`could not start ${suite.cmd}: ${result.error.message}`);
    process.exit(1);
  }
  if (result.status !== 0) {
    console.error(`\nFAILED: ${suite.name}`);
    process.exit(result.status ?? 1);
  }
}

console.log("\nAll test suites passed.");
