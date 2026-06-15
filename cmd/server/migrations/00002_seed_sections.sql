-- +goose Up

INSERT INTO sections (name, sort_order) VALUES
    ('Produce',               1),
    ('Dairy & Eggs',          2),
    ('Meat & Seafood',        3),
    ('Bakery & Bread',        4),
    ('Frozen Foods',          5),
    ('Pantry & Dry Goods',    6),
    ('Beverages',             7),
    ('Snacks & Sweets',       8),
    ('Personal Care',         9),
    ('Household & Cleaning', 10),
    ('Other',                11)
ON CONFLICT (name) DO NOTHING;

-- +goose Down

DELETE FROM sections WHERE name IN (
    'Produce', 'Dairy & Eggs', 'Meat & Seafood', 'Bakery & Bread',
    'Frozen Foods', 'Pantry & Dry Goods', 'Beverages', 'Snacks & Sweets',
    'Personal Care', 'Household & Cleaning', 'Other'
);
