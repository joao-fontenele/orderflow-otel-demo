CREATE TABLE inventory.items (
    item_id VARCHAR PRIMARY KEY,
    name VARCHAR NOT NULL,
    available INTEGER NOT NULL DEFAULT 0,
    reserved INTEGER NOT NULL DEFAULT 0
);

INSERT INTO inventory.items (item_id, name, available, reserved) VALUES
    ('ITEM-001', 'Wireless Mouse', 100, 0),
    ('ITEM-002', 'Mechanical Keyboard', 50, 0),
    ('ITEM-003', 'USB-C Hub', 200, 0),
    ('ITEM-004', 'Monitor Stand', 75, 0),
    ('ITEM-005', 'Webcam HD', 30, 0),
    ('ITEM-006', 'Headset Pro', 150, 0),
    ('ITEM-007', 'Mouse Pad XL', 300, 0),
    ('ITEM-008', 'Cable Management Kit', 120, 0),
    ('ITEM-009', 'Laptop Stand', 80, 0),
    ('ITEM-010', 'Desk Lamp LED', 60, 0);
