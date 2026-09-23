import { createPostgresApp, createMongoApp } from "./utils/bootstrap.js";

const appFactories = {
  postgres: () => createPostgresApp(),
  mongo: () => createMongoApp(),
};

const DB = process.env.DB || "postgres";
const app = await appFactories[DB]();

const PORT = process.env.PORT || 3000;
app.listen(PORT, () => {
  console.log(`Server listening on port ${PORT}`);
});
