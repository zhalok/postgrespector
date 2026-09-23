import { pool } from "../db-clients/postgres.js";

export async function createOrder({
  order_id,
  status,
  customer,
  financials,
  line_items,
  polymorphic_metadata,
  event,
}) {
  await pool.query(
    `INSERT INTO orders
      (order_id, "timestamp", status, customer, financials, line_items, polymorphic_metadata, event_timeline)
     VALUES ($1, now(), $2, $3::jsonb, $4::jsonb, $5::jsonb, $6::jsonb, $7::jsonb)`,
    [
      order_id,
      status,
      customer ? JSON.stringify(customer) : null,
      financials ? JSON.stringify(financials) : null,
      line_items ? JSON.stringify(line_items) : null,
      polymorphic_metadata ? JSON.stringify(polymorphic_metadata) : null,
      JSON.stringify([event]),
    ]
  );
}

export async function getOrder(orderId) {
  const { rows } = await pool.query(
    `SELECT order_id, status FROM orders WHERE order_id = $1`,
    [orderId]
  );
  return rows[0] ?? null;
}

export async function addLineItems(orderId, lineItems, event) {
  const { rows } = await pool.query(
    `UPDATE orders
     SET line_items = COALESCE(line_items, '[]'::jsonb) || $2::jsonb,
         event_timeline = event_timeline || $3::jsonb
     WHERE order_id = $1
     RETURNING order_id, line_items, event_timeline`,
    [orderId, JSON.stringify(lineItems), JSON.stringify([event])]
  );
  return rows[0] ?? null;
}

export async function transitionStatus(orderId, expectedStatus, newStatus, event) {
  const { rows } = await pool.query(
    `UPDATE orders
     SET status = $2, event_timeline = event_timeline || $3::jsonb
     WHERE order_id = $1 AND status = $4
     RETURNING order_id, status, event_timeline`,
    [orderId, newStatus, JSON.stringify([event]), expectedStatus]
  );
  return rows[0] ?? null;
}

const PAGE_SIZE = 1000;

export async function getOrders(cursor, status) {
  const { rows } = await pool.query(
    `SELECT
       order_id,
       "timestamp",
       jsonb_build_object(
         'id', customer->>'id',
         'geo_location', jsonb_build_object(
           'country', customer#>>'{session_context,geo_location,country}'
         )
       ) AS customer,
       jsonb_build_object(
         'final_total', (financials#>>'{amounts,final_total}')::numeric,
         'currency', financials->>'currency'
       ) AS financials
     FROM orders
     WHERE ($1::timestamptz IS NULL OR "timestamp" > $1)
       AND ($2::text IS NULL OR status = $2)
     ORDER BY "timestamp" ASC
     LIMIT ${PAGE_SIZE}`,
    [cursor ?? null, status ?? null]
  );

  const next_cursor = rows.length === PAGE_SIZE ? rows[rows.length - 1].timestamp : null;
  return { orders: rows, next_cursor };
}

export const ordersRepositoryPostgres = {
  createOrder,
  getOrder,
  getOrders,
  addLineItems,
  transitionStatus,
};
