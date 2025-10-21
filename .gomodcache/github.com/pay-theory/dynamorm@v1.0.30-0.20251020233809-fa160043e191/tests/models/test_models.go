package models

import "time"

// TestUser is a test model for user data
type TestUser struct {
	ID        string    `dynamorm:"pk"`
	Email     string    `dynamorm:"sk,index:gsi-email,pk"`
	CreatedAt time.Time `dynamorm:"index:gsi-email,sk"`
	Age       int       `dynamorm:""`
	Status    string    `dynamorm:""`
	Tags      []string  `dynamorm:""`
	Name      string    `dynamorm:""`
}

// TestProduct is a test model for product data
type TestProduct struct {
	SKU         string    `dynamorm:"pk"`
	Category    string    `dynamorm:"sk,index:gsi-category,pk"`
	Price       float64   `dynamorm:"index:gsi-category,sk"`
	Name        string    `dynamorm:""`
	Description string    `dynamorm:""`
	InStock     bool      `dynamorm:""`
	CreatedAt   time.Time `dynamorm:""`
}

// TestOrder is a test model for complex queries
type TestOrder struct {
	OrderID    string      `dynamorm:"pk"`
	CustomerID string      `dynamorm:"sk,index:gsi-customer,pk"`
	Status     string      `dynamorm:"index:gsi-status,pk"`
	Total      float64     `dynamorm:"index:gsi-status,sk"`
	Items      []OrderItem `dynamorm:""`
	CreatedAt  time.Time   `dynamorm:"index:gsi-customer,sk"`
	UpdatedAt  time.Time   `dynamorm:""`
}

// OrderItem represents an item in an order
type OrderItem struct {
	ProductSKU string  `dynamorm:""`
	Quantity   int     `dynamorm:""`
	Price      float64 `dynamorm:""`
}
