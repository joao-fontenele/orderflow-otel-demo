CREATE TABLE orders.orders (
    id UUID PRIMARY KEY,
    customer_id VARCHAR NOT NULL,
    status VARCHAR NOT NULL,
    total BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE orders.order_items (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders.orders(id) ON DELETE CASCADE,
    item_id VARCHAR NOT NULL,
    quantity INTEGER NOT NULL,
    price BIGINT NOT NULL
);

CREATE INDEX idx_order_items_order_id ON orders.order_items(order_id);
CREATE INDEX idx_orders_customer_id ON orders.orders(customer_id);
