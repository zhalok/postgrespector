// Assembles decomposed dashboard sources (monitoring/grafana/dashboards/<name>/) into
// the single-file JSON that Grafana's file provisioning reads from
// monitoring/grafana/dashboards/<name>.json. Run after editing any panel file.
import { readFileSync, readdirSync, writeFileSync, statSync } from "fs";
import { join, basename } from "path";

const dashboardsDir = new URL("dashboards/", import.meta.url).pathname;

const sourceDirs = readdirSync(dashboardsDir).filter((entry) =>
  statSync(join(dashboardsDir, entry)).isDirectory()
);

for (const name of sourceDirs) {
  const srcDir = join(dashboardsDir, name);
  const dashboard = JSON.parse(readFileSync(join(srcDir, "dashboard.json"), "utf8"));

  const panelsDir = join(srcDir, "panels");
  const elements = {};
  for (const file of readdirSync(panelsDir).sort()) {
    const panelName = basename(file, ".json");
    elements[panelName] = JSON.parse(readFileSync(join(panelsDir, file), "utf8"));
  }

  dashboard.spec.elements = elements;

  const outPath = join(dashboardsDir, `${name}.json`);
  writeFileSync(outPath, JSON.stringify(dashboard, null, 2) + "\n");
  console.log(`built ${outPath}`);
}
