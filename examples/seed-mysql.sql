-- Sample MySQL seed data for Faucet demo

CREATE TABLE products (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    price DECIMAL(10,2) NOT NULL,
    category VARCHAR(100),
    in_stock BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE orders (
    id INT AUTO_INCREMENT PRIMARY KEY,
    customer_email VARCHAR(255) NOT NULL,
    total DECIMAL(10,2) NOT NULL,
    status ENUM('pending', 'shipped', 'delivered', 'cancelled') DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE order_items (
    id INT AUTO_INCREMENT PRIMARY KEY,
    order_id INT,
    product_id INT,
    quantity INT NOT NULL DEFAULT 1,
    unit_price DECIMAL(10,2) NOT NULL,
    FOREIGN KEY (order_id) REFERENCES orders(id),
    FOREIGN KEY (product_id) REFERENCES products(id)
);

-- Seed data
INSERT INTO products (name, price, category) VALUES
    ('Widget A', 9.99, 'widgets'),
    ('Widget B', 19.99, 'widgets'),
    ('Gadget X', 49.99, 'gadgets'),
    ('Gadget Y', 99.99, 'gadgets');

INSERT INTO orders (customer_email, total, status) VALUES
    ('buyer@example.com', 29.98, 'shipped'),
    ('buyer@example.com', 99.99, 'pending');

INSERT INTO order_items (order_id, product_id, quantity, unit_price) VALUES
    (1, 1, 2, 9.99),
    (1, 2, 1, 19.99),
    (2, 4, 1, 99.99);
