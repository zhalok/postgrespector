export async function createOrderHandler(req, res, repository) {
  const { order_id, status, customer, financials, line_items, polymorphic_metadata } = req.body;

  if (!order_id || !status) {
    return res.status(400).json({ error: "order_id and status are required" });
  }

  const cartCreatedEvent = {
    event: "cart_created",
    timestamp: new Date().toISOString(),
    payload: {},
  };

  try {
    await repository.createOrder({
      order_id,
      status,
      customer,
      financials,
      line_items,
      polymorphic_metadata,
      event: cartCreatedEvent,
    });

    res.status(201).json({ order_id, event_timeline: [cartCreatedEvent] });
  } catch (err) {
    console.error("Failed to create order", err);
    res.status(500).json({ error: "Failed to create order" });
  }
}

export async function getOrdersHandler(req, res, repository) {
  const { cursor, status } = req.query;

  try {
    const { orders, next_cursor } = await repository.getOrders(cursor ?? null, status ?? null);
    res.status(200).json({ orders, next_cursor });
  } catch (err) {
    console.error("Failed to fetch orders", err);
    res.status(500).json({ error: "Failed to fetch orders" });
  }
}

export async function addItemsHandler(req, res, repository) {
  const { orderId } = req.params;
  const { line_items } = req.body;

  if (!Array.isArray(line_items) || line_items.length === 0) {
    return res.status(400).json({ error: "line_items must be a non-empty array" });
  }

  const order = await repository.getOrder(orderId);
  if (!order) {
    return res.status(404).json({ error: "Order not found" });
  }

  const event = {
    event: "items_added",
    timestamp: new Date().toISOString(),
    payload: { items: line_items.map((item) => item.sku ?? item.item_id) },
  };

  try {
    const updated = await repository.addLineItems(orderId, line_items, event);
    res.status(200).json(updated);
  } catch (err) {
    console.error("Failed to add items", err);
    res.status(500).json({ error: "Failed to add items" });
  }
}

export async function makePaymentHandler(req, res, repository) {
  const { orderId } = req.params;

  const event = {
    event: "payment_initiated",
    timestamp: new Date().toISOString(),
    payload: { method: req.body?.method ?? "unknown" },
  };

  try {
    const updated = await repository.transitionStatus(
      orderId,
      "PLACED",
      "PAYMENT_PROCESSING",
      event
    );
    if (!updated) {
      const order = await repository.getOrder(orderId);
      if (!order) return res.status(404).json({ error: "Order not found" });
      return res.status(409).json({
        error: `Cannot start payment from status "${order.status}"`,
      });
    }
    res.status(200).json(updated);
  } catch (err) {
    console.error("Failed to start payment", err);
    res.status(500).json({ error: "Failed to start payment" });
  }
}

export async function paymentWebhookHandler(req, res, repository) {
  const { orderId } = req.params;

  const event = {
    event: "payment_successful",
    timestamp: new Date().toISOString(),
    payload: { transaction_id: req.body?.transaction_id ?? null },
  };

  try {
    const updated = await repository.transitionStatus(
      orderId,
      "PAYMENT_PROCESSING",
      "PAYMENT_SUCCESSFUL",
      event
    );
    if (!updated) {
      const order = await repository.getOrder(orderId);
      if (!order) return res.status(404).json({ error: "Order not found" });
      return res.status(409).json({
        error: `Cannot confirm payment from status "${order.status}"`,
      });
    }
    res.status(200).json(updated);
  } catch (err) {
    console.error("Failed to confirm payment", err);
    res.status(500).json({ error: "Failed to confirm payment" });
  }
}

export async function startDeliveryHandler(req, res, repository) {
  const { orderId } = req.params;

  const event = {
    event: "delivery_started",
    timestamp: new Date().toISOString(),
    payload: {
      carrier: req.body?.carrier ?? "unknown",
      tracking_number: req.body?.tracking_number ?? null,
    },
  };

  try {
    const updated = await repository.transitionStatus(
      orderId,
      "PAYMENT_SUCCESSFUL",
      "DELIVERING",
      event
    );
    if (!updated) {
      const order = await repository.getOrder(orderId);
      if (!order) return res.status(404).json({ error: "Order not found" });
      return res.status(409).json({
        error: `Cannot start delivery from status "${order.status}"`,
      });
    }
    res.status(200).json(updated);
  } catch (err) {
    console.error("Failed to start delivery", err);
    res.status(500).json({ error: "Failed to start delivery" });
  }
}

export async function completeOrderHandler(req, res, repository) {
  const { orderId } = req.params;

  const event = {
    event: "order_completed",
    timestamp: new Date().toISOString(),
    payload: {},
  };

  try {
    const updated = await repository.transitionStatus(orderId, "DELIVERING", "COMPLETED", event);
    if (!updated) {
      const order = await repository.getOrder(orderId);
      if (!order) return res.status(404).json({ error: "Order not found" });
      return res.status(409).json({
        error: `Cannot complete order from status "${order.status}"`,
      });
    }
    res.status(200).json(updated);
  } catch (err) {
    console.error("Failed to complete order", err);
    res.status(500).json({ error: "Failed to complete order" });
  }
}
