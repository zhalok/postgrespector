import pg from "pg";

const { Client } = pg;

const PAGE_SIZE = 100;

function makeClient() {
  return new Client({
    host: process.env.PGHOST || "192.168.10.238",
    port: process.env.PGPORT ? Number(process.env.PGPORT) : 5432,
    user: process.env.PGUSER || "postgres",
    password: process.env.PGPASSWORD || "postgres",
    database: process.env.PGDATABASE || "tpch",
  });
}

async function findLargestTable(client) {
  const { rows } = await client.query(`
    select schemaname, relname
    from pg_catalog.pg_statio_user_tables
    order by pg_total_relation_size(relid) desc
    limit 1
  `);

  if (rows.length === 0) {
    throw new Error("No user tables found in database");
  }

  return rows[0];
}

async function scanTableWithOffsetPagination(client, schemaname, relname) {
  const qualifiedName = `"${schemaname}"."${relname}"`;

  let rowCount = 0;
  let offset = 0;
  let page = 0;

  while (true) {
    page += 1;
    const { rows } = await client.query(
      `select * from ${qualifiedName} limit $1 offset $2`,
      [PAGE_SIZE, offset]
    );

    console.log(`  page ${page} - ${rows.length} rows`);

    rowCount += rows.length;
    if (rows.length === 0) {
      break;
    }

    offset += rows.length;
    if (rows.length < PAGE_SIZE) {
      break;
    }
  }

  return rowCount;
}

async function main() {
  const client = makeClient();
  await client.connect();
  console.log("Connected to postgres");

  let stopping = false;
  const stop = () => {
    stopping = true;
  };
  process.on("SIGINT", stop);
  process.on("SIGTERM", stop);

  try {
    let iteration = 0;
    while (!stopping) {
      iteration += 1;

      const { schemaname, relname } = await findLargestTable(client);

      const start = Date.now();
      const rowCount = await scanTableWithOffsetPagination(client, schemaname, relname);
      const elapsedMs = Date.now() - start;

      console.log(
        `[iteration ${iteration}] scanned "${schemaname}"."${relname}" - ${rowCount} rows in ${elapsedMs}ms`
      );
    }
  } finally {
    await client.end();
    console.log("Disconnected, stopped");
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
