import { db } from "../db-clients/mongo.js";

const orders = db.collection("orders");
await orders.createIndex({ status: 1 });

export async function createOrder({
  order_id,
  status,
  customer,
  financials,
  line_items,
  polymorphic_metadata,
  event,
}) {
  await orders.insertOne({
    order_id,
    timestamp: new Date(),
    status,
    customer: customer ?? null,
    financials: financials ?? null,
    line_items: line_items ?? [],
    polymorphic_metadata: polymorphic_metadata ?? null,
    event_timeline: [event],
  });
}

export async function getOrder(orderId) {
  return orders.findOne(
    { order_id: orderId },
    { projection: { _id: 0, order_id: 1, status: 1 } }
  );
}

export async function addLineItems(orderId, lineItems, event) {
  const result = await orders.findOneAndUpdate(
    { order_id: orderId },
    {
      $push: {
        line_items: { $each: lineItems },
        event_timeline: { $each: [event] },
      },
    },
    {
      returnDocument: "after",
      projection: { _id: 0, order_id: 1, line_items: 1, event_timeline: 1 },
    }
  );
  return result ?? null;
}

export async function transitionStatus(orderId, expectedStatus, newStatus, event) {
  const result = await orders.findOneAndUpdate(
    { order_id: orderId, status: expectedStatus },
    {
      $set: { status: newStatus },
      $push: { event_timeline: event },
    },
    {
      returnDocument: "after",
      projection: { _id: 0, order_id: 1, status: 1, event_timeline: 1 },
    }
  );
  return result ?? null;
}

const PAGE_SIZE = 1000;

export async function getOrders(cursor, status) {
  const match = {};
  if (cursor) match.timestamp = { $gt: new Date(cursor) };
  if (status) match.status = status;

  const rows = await orders
    .aggregate([
      { $match: match },
      { $sort: { timestamp: 1 } },
      { $limit: PAGE_SIZE },
      {
        $project: {
          _id: 0,
          order_id: 1,
          timestamp: 1,
          customer: {
            id: "$customer.id",
            geo_location: { country: "$customer.session_context.geo_location.country" },
          },
          financials: {
            final_total: "$financials.amounts.final_total",
            currency: "$financials.currency",
          },
        },
      },
    ])
    .toArray();

  const next_cursor = rows.length === PAGE_SIZE ? rows[rows.length - 1].timestamp : null;
  return { orders: rows, next_cursor };
}

export const ordersRepositoryMongodb = {
  createOrder,
  getOrder,
  getOrders,
  addLineItems,
  transitionStatus,
};
