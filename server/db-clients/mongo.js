import { MongoClient } from "mongodb";

const client = new MongoClient(process.env.MONGO_URI || "mongodb://mongo:mongo@localhost:27017");

await client.connect();

export const db = client.db(process.env.MONGO_DB || "jsonb_experiments");
