// Reverse of build-dashboards.js: splits a single-file dashboard
// (monitoring/grafana/dashboards/<name>.json, e.g. exported from the Grafana UI)
// back into decomposed sources under monitoring/grafana/dashboards/<name>/
// (dashboard.json + panels/*.json). Run after exporting a dashboard from Grafana.
import { readFileSync, readdirSync, writeFileSync, mkdirSync, statSync } from "fs";
import { join, basename } from "path";

const dashboardsDir = new URL("dashboards/", import.meta.url).pathname;

const sourceFiles = readdirSync(dashboardsDir).filter(
  (entry) => entry.endsWith(".json") && statSync(join(dashboardsDir, entry)).isFile()
);

for (const file of sourceFiles) {
  const name = basename(file, ".json");
  const dashboard = JSON.parse(readFileSync(join(dashboardsDir, file), "utf8"));

  if (!dashboard.spec?.elements) {
    console.log(`skipped ${file} (no spec.elements, not a decomposable v2 dashboard)`);
    continue;
  }

  const elements = dashboard.spec.elements;
  delete dashboard.spec.elements;

  const srcDir = join(dashboardsDir, name);
  const panelsDir = join(srcDir, "panels");
  mkdirSync(panelsDir, { recursive: true });

  for (const [panelName, panel] of Object.entries(elements)) {
    writeFileSync(join(panelsDir, `${panelName}.json`), JSON.stringify(panel, null, 2) + "\n");
  }

  writeFileSync(join(srcDir, "dashboard.json"), JSON.stringify(dashboard, null, 2) + "\n");
  console.log(`decomposed ${file} -> ${srcDir}/`);
}
