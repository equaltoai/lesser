package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// Core e-commerce domain models with multi-tenant architecture

// Tenant represents a multi-tenant e-commerce store
// Memory optimized: 512 → 504 bytes (8 bytes saved)
type Tenant struct {
	CreatedAt     time.Time    `json:"createdAt"`
	UpdatedAt     time.Time    `json:"updatedAt"`
	Owner         TenantOwner  `json:"owner"`
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Domain        string       `json:"domain"`
	Subscription  Subscription `json:"subscription"`
	Configuration TenantConfig `json:"configuration"`
	IsActive      bool         `json:"isActive"`
}

// TenantConfig holds tenant-specific configuration
// Memory optimized: 216 → 152 bytes (64 bytes saved)
type TenantConfig struct {
	CustomSettings  map[string]any `json:"customSettings"`
	Theme           ThemeConfig    `json:"theme"`
	Currency        string         `json:"currency"`
	Locale          string         `json:"locale"`
	PaymentMethods  []string       `json:"paymentMethods"`
	ShippingMethods []string       `json:"shippingMethods"`
	Limits          TenantLimits   `json:"limits"`
	Features        FeatureFlags   `json:"features"`
}

// ThemeConfig defines the visual appearance
type ThemeConfig struct {
	PrimaryColor   string `json:"primaryColor"`
	SecondaryColor string `json:"secondaryColor"`
	Logo           string `json:"logo"`
	Favicon        string `json:"favicon"`
	CustomCSS      string `json:"customCss"`
}

// FeatureFlags control tenant features
type FeatureFlags struct {
	AdvancedSearch    bool `json:"advancedSearch"`
	ProductReviews    bool `json:"productReviews"`
	WishList          bool `json:"wishList"`
	Recommendations   bool `json:"recommendations"`
	MultiCurrency     bool `json:"multiCurrency"`
	InventoryTracking bool `json:"inventoryTracking"`
	Analytics         bool `json:"analytics"`
}

// TenantLimits define usage limits
type TenantLimits struct {
	MaxProducts     int `json:"maxProducts"`
	MaxOrders       int `json:"maxOrders"`
	MaxCustomers    int `json:"maxCustomers"`
	StorageLimit    int `json:"storageLimit"`   // in MB
	BandwidthLimit  int `json:"bandwidthLimit"` // in GB
	APICallsPerHour int `json:"apiCallsPerHour"`
}

// Subscription represents tenant subscription
type Subscription struct {
	Plan         string    `json:"plan"`
	Status       string    `json:"status"`
	StartDate    time.Time `json:"startDate"`
	EndDate      time.Time `json:"endDate"`
	BillingCycle string    `json:"billingCycle"`
	Amount       Money     `json:"amount"`
}

// TenantOwner represents the tenant owner
type TenantOwner struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Company string `json:"company"`
}

// Product represents a product in the catalog
// Memory optimized: 384 → 336 bytes (48 bytes saved)
type Product struct {
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
	Attributes   map[string]any   `json:"attributes"`
	ComparePrice *Money           `json:"comparePrice,omitempty"`
	ID           string           `json:"id"`
	Status       ProductStatus    `json:"status"`
	Description  string           `json:"description"`
	Name         string           `json:"name"`
	SKU          string           `json:"sku"`
	TenantID     string           `json:"tenantId"`
	SEO          SEOData          `json:"seo"`
	Categories   []string         `json:"categories"`
	Price        Money            `json:"price"`
	Tags         []string         `json:"tags"`
	Variants     []ProductVariant `json:"variants,omitempty"`
	Images       []ProductImage   `json:"images"`
	Inventory    Inventory        `json:"inventory"`
}

// Money represents monetary values
type Money struct {
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// Inventory tracks product availability
type Inventory struct {
	Quantity          int  `json:"quantity"`
	Reserved          int  `json:"reserved"`
	Available         int  `json:"available"`
	TrackStock        bool `json:"trackStock"`
	AllowBackorder    bool `json:"allowBackorder"`
	LowStockThreshold int  `json:"lowStockThreshold"`
}

// ProductImage represents product images
type ProductImage struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	AltText   string `json:"altText"`
	Position  int    `json:"position"`
	IsPrimary bool   `json:"isPrimary"`
}

// SEOData for search engine optimization
type SEOData struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Slug        string   `json:"slug"`
	Keywords    []string `json:"keywords"`
}

// ProductStatus represents product status
type ProductStatus string

const (
	ProductStatusActive   ProductStatus = "active"
	ProductStatusInactive ProductStatus = "inactive"
	ProductStatusDraft    ProductStatus = "draft"
	ProductStatusArchived ProductStatus = "archived"
)

// ProductVariant represents product variations
type ProductVariant struct {
	Attributes map[string]string `json:"attributes"`
	ID         string            `json:"id"`
	SKU        string            `json:"sku"`
	Name       string            `json:"name"`
	Price      Money             `json:"price"`
	Inventory  Inventory         `json:"inventory"`
}

// Customer represents a customer
// Memory optimized: 384 → 368 bytes (16 bytes saved)
type Customer struct {
	CreatedAt      time.Time           `json:"createdAt"`
	LastLoginAt    time.Time           `json:"lastLoginAt"`
	UpdatedAt      time.Time           `json:"updatedAt"`
	Profile        CustomerProfile     `json:"profile"`
	Email          string              `json:"email"`
	TenantID       string              `json:"tenantId"`
	ID             string              `json:"id"`
	Tags           []string            `json:"tags"`
	OrderHistory   []string            `json:"orderHistory"`
	PaymentMethods []PaymentMethod     `json:"paymentMethods"`
	Addresses      []Address           `json:"addresses"`
	Preferences    CustomerPreferences `json:"preferences"`
	IsActive       bool                `json:"isActive"`
}

// CustomerProfile contains customer personal information
type CustomerProfile struct {
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	Phone       string    `json:"phone"`
	DateOfBirth time.Time `json:"dateOfBirth,omitempty"`
	Gender      string    `json:"gender,omitempty"`
	Avatar      string    `json:"avatar,omitempty"`
}

// Address represents shipping/billing addresses
type Address struct {
	ID         string `json:"id"`
	Type       string `json:"type"` // shipping, billing
	FirstName  string `json:"firstName"`
	LastName   string `json:"lastName"`
	Company    string `json:"company,omitempty"`
	Address1   string `json:"address1"`
	Address2   string `json:"address2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postalCode"`
	Country    string `json:"country"`
	Phone      string `json:"phone,omitempty"`
	IsDefault  bool   `json:"isDefault"`
}

// PaymentMethod represents customer payment methods
// Memory optimized for better alignment
type PaymentMethod struct {
	// Time struct first (24 bytes)
	CreatedAt time.Time `json:"createdAt"`
	// Strings (16 bytes each)
	ID       string `json:"id"`
	Type     string `json:"type"` // card, bank, wallet
	Provider string `json:"provider"`
	Last4    string `json:"last4,omitempty"`
	// Ints (4 bytes each)
	ExpiryMonth int `json:"expiryMonth,omitempty"`
	ExpiryYear  int `json:"expiryYear,omitempty"`
	// Bool last (1 byte)
	IsDefault bool `json:"isDefault"`
}

// CustomerPreferences stores customer preferences
// Memory optimized: 48 → 40 bytes (8 bytes saved)
type CustomerPreferences struct {
	Language           string   `json:"language"`
	Currency           string   `json:"currency"`
	FavoriteCategories []string `json:"favoriteCategories"`
	EmailMarketing     bool     `json:"emailMarketing"`
	SMSMarketing       bool     `json:"smsMarketing"`
	PushNotifications  bool     `json:"pushNotifications"`
}

// Order represents a customer order
// Memory optimized: 720 → 704 bytes (16 bytes saved)
type Order struct {
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	CompletedAt *time.Time   `json:"completedAt,omitempty"`
	CancelledAt *time.Time   `json:"canceledAt,omitempty"`
	Shipping    ShippingInfo `json:"shipping"`
	Payment     PaymentInfo  `json:"payment"`
	ID          string       `json:"id"`
	TenantID    string       `json:"tenantId"`
	CustomerID  string       `json:"customerId"`
	OrderNumber string       `json:"orderNumber"`
	Notes       string       `json:"notes,omitempty"`
	Status      OrderStatus  `json:"status"`
	Items       []OrderItem  `json:"items"`
	Totals      OrderTotals  `json:"totals"`
}

// OrderItem represents items in an order
type OrderItem struct {
	Attributes map[string]string `json:"attributes,omitempty"`
	ID         string            `json:"id"`
	ProductID  string            `json:"productId"`
	VariantID  string            `json:"variantId,omitempty"`
	SKU        string            `json:"sku"`
	Name       string            `json:"name"`
	Price      Money             `json:"price"`
	Total      Money             `json:"total"`
	Quantity   int               `json:"quantity"`
}

// OrderTotals represents order financial totals
type OrderTotals struct {
	Subtotal Money   `json:"subtotal"`
	Tax      Money   `json:"tax"`
	Shipping Money   `json:"shipping"`
	Discount Money   `json:"discount"`
	Total    Money   `json:"total"`
	TaxRate  float64 `json:"taxRate"`
}

// PaymentInfo represents payment information
// Memory optimized: 280 → 272 bytes (8 bytes saved)
type PaymentInfo struct {
	ProcessedAt   time.Time  `json:"processedAt"`
	RefundedAt    *time.Time `json:"refundedAt,omitempty"`
	RefundAmount  *Money     `json:"refundAmount,omitempty"`
	Method        string     `json:"method"`
	Provider      string     `json:"provider"`
	TransactionID string     `json:"transactionId"`
	Status        string     `json:"status"`
	Amount        Money      `json:"amount"`
}

// ShippingInfo represents shipping information
// Memory optimized: 240 → 224 bytes (16 bytes saved)
type ShippingInfo struct {
	EstimatedDelivery time.Time  `json:"estimatedDelivery"`
	ShippedAt         *time.Time `json:"shippedAt,omitempty"`
	DeliveredAt       *time.Time `json:"deliveredAt,omitempty"`
	Method            string     `json:"method"`
	Provider          string     `json:"provider"`
	TrackingNumber    string     `json:"trackingNumber,omitempty"`
	Address           Address    `json:"address"`
}

// OrderStatus represents order status
type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusConfirmed  OrderStatus = "confirmed"
	OrderStatusProcessing OrderStatus = "processing"
	OrderStatusShipped    OrderStatus = "shipped"
	OrderStatusDelivered  OrderStatus = "delivered"
	OrderStatusCancelled  OrderStatus = "canceled"
	OrderStatusRefunded   OrderStatus = "refunded"
)

// ShoppingCart represents a customer's shopping cart
// Memory optimized for better alignment
type ShoppingCart struct {
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	ID         string     `json:"id"`
	TenantID   string     `json:"tenantId"`
	CustomerID string     `json:"customerId"`
	Items      []CartItem `json:"items"`
	Totals     CartTotals `json:"totals"`
}

// CartItem represents items in shopping cart
// Memory optimized for better alignment
type CartItem struct {
	AddedAt   time.Time `json:"addedAt"`
	ID        string    `json:"id"`
	ProductID string    `json:"productId"`
	VariantID string    `json:"variantId,omitempty"`
	Price     Money     `json:"price"`
	Total     Money     `json:"total"`
	Quantity  int       `json:"quantity"`
}

// CartTotals represents cart totals
type CartTotals struct {
	Subtotal  Money `json:"subtotal"`
	Tax       Money `json:"tax"`
	Shipping  Money `json:"shipping"`
	Total     Money `json:"total"`
	ItemCount int   `json:"itemCount"`
}

// Request/Response models
// Memory optimized: 344 → 288 bytes (56 bytes saved)
type CreateTenantRequest struct {
	Owner         TenantOwner  `json:"owner" validate:"required"`
	Name          string       `json:"name" validate:"required"`
	Domain        string       `json:"domain" validate:"required"`
	Plan          string       `json:"plan" validate:"required"`
	Configuration TenantConfig `json:"configuration"`
}

// Memory optimized: 248 → 208 bytes (40 bytes saved)
type CreateProductRequest struct {
	Attributes  map[string]any `json:"attributes"`
	SKU         string         `json:"sku" validate:"required"`
	Name        string         `json:"name" validate:"required"`
	Description string         `json:"description"`
	SEO         SEOData        `json:"seo"`
	Images      []ProductImage `json:"images"`
	Categories  []string       `json:"categories"`
	Tags        []string       `json:"tags"`
	Price       Money          `json:"price" validate:"required"`
	Inventory   Inventory      `json:"inventory"`
}

type CreateCustomerRequest struct {
	Email       string              `json:"email" validate:"required,email"`
	Profile     CustomerProfile     `json:"profile" validate:"required"`
	Addresses   []Address           `json:"addresses"`
	Preferences CustomerPreferences `json:"preferences"`
}

// Memory optimized: 464 → 456 bytes (8 bytes saved)
type CreateOrderRequest struct {
	Shipping   ShippingInfo `json:"shipping" validate:"required"`
	Payment    PaymentInfo  `json:"payment" validate:"required"`
	CustomerID string       `json:"customerId" validate:"required"`
	Notes      string       `json:"notes"`
	Items      []OrderItem  `json:"items" validate:"required,min=1"`
}

type AddToCartRequest struct {
	ProductID string `json:"productId" validate:"required"`
	VariantID string `json:"variantId"`
	Quantity  int    `json:"quantity" validate:"required,min=1"`
}

type UpdateCartItemRequest struct {
	Quantity int `json:"quantity" validate:"required,min=0"`
}

// Service interfaces
type TenantService interface {
	// CreateTenant creates a new tenant with the given request.
	// Parameters:
	//   - ctx: The context for the request
	//   - req: The request to create a tenant
	// Returns:
	//   - The created tenant
	//   - An error if the creation fails
	CreateTenant(ctx context.Context, req CreateTenantRequest) (*Tenant, error)

	// GetTenant retrieves a tenant by their ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the tenant
	// Returns:
	//   - The retrieved tenant
	//   - An error if the retrieval fails
	GetTenant(ctx context.Context, id string) (*Tenant, error)

	// GetTenantByDomain retrieves a tenant by their domain.
	// Parameters:
	//   - ctx: The context for the request
	//   - domain: The domain of the tenant
	// Returns:
	//   - The retrieved tenant
	//   - An error if the retrieval fails
	GetTenantByDomain(ctx context.Context, domain string) (*Tenant, error)

	// UpdateTenant updates a tenant's information.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the tenant
	//   - tenant: The updated tenant information
	// Returns:
	//   - An error if the update fails
	UpdateTenant(ctx context.Context, id string, tenant *Tenant) error

	// ListTenants lists all tenants with pagination.
	// Parameters:
	//   - ctx: The context for the request
	//   - limit: The maximum number of tenants to retrieve
	//   - offset: The offset for pagination
	// Returns:
	//   - A list of tenants
	//   - An error if the retrieval fails
	ListTenants(ctx context.Context, limit, offset int) ([]Tenant, error)

	// DeactivateTenant deactivates a tenant by their ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the tenant
	// Returns:
	//   - An error if the deactivation fails
	DeactivateTenant(ctx context.Context, id string) error
}

type ProductService interface {
	// CreateProduct creates a new product with the given request.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - req: The request to create a product
	// Returns:
	//   - The created product
	//   - An error if the creation fails
	CreateProduct(ctx context.Context, tenantID string, req CreateProductRequest) (*Product, error)

	// GetProduct retrieves a product by its ID and tenant ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - id: The ID of the product
	// Returns:
	//   - The retrieved product
	//   - An error if the retrieval fails
	GetProduct(ctx context.Context, tenantID, id string) (*Product, error)

	// UpdateProduct updates a product's information.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - id: The ID of the product
	//   - product: The updated product information
	// Returns:
	//   - An error if the update fails
	UpdateProduct(ctx context.Context, tenantID, id string, product *Product) error

	// DeleteProduct deletes a product by its ID and tenant ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - id: The ID of the product
	// Returns:
	//   - An error if the deletion fails
	DeleteProduct(ctx context.Context, tenantID, id string) error

	// ListProducts lists all products for a tenant with optional filters.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - filters: Optional filters for the products
	// Returns:
	//   - A list of products
	//   - An error if the retrieval fails
	ListProducts(ctx context.Context, tenantID string, filters ProductFilters) ([]Product, error)

	// SearchProducts searches for products based on a query and optional filters.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - query: The search query
	//   - filters: Optional filters for the products
	// Returns:
	//   - A list of products matching the query
	//   - An error if the search fails
	SearchProducts(ctx context.Context, tenantID, query string, filters ProductFilters) ([]Product, error)

	// UpdateInventory updates the inventory quantity of a product.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - productID: The ID of the product
	//   - quantity: The new quantity
	// Returns:
	//   - An error if the update fails
	UpdateInventory(ctx context.Context, tenantID, productID string, quantity int) error
}

type CustomerService interface {
	// CreateCustomer creates a new customer with the given request.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - req: The request to create a customer
	// Returns:
	//   - The created customer
	//   - An error if the creation fails
	CreateCustomer(ctx context.Context, tenantID string, req CreateCustomerRequest) (*Customer, error)

	// GetCustomer retrieves a customer by their ID and tenant ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - id: The ID of the customer
	// Returns:
	//   - The retrieved customer
	//   - An error if the retrieval fails
	GetCustomer(ctx context.Context, tenantID, id string) (*Customer, error)

	// UpdateCustomer updates a customer's information.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - id: The ID of the customer
	//   - customer: The updated customer information
	// Returns:
	//   - An error if the update fails
	UpdateCustomer(ctx context.Context, tenantID, id string, customer *Customer) error

	// ListCustomers lists all customers for a tenant with pagination.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - limit: The maximum number of customers to retrieve
	//   - offset: The offset for pagination
	// Returns:
	//   - A list of customers
	//   - An error if the retrieval fails
	ListCustomers(ctx context.Context, tenantID string, limit, offset int) ([]Customer, error)

	// AuthenticateCustomer authenticates a customer by their email and password.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - email: The email of the customer
	//   - password: The password of the customer
	// Returns:
	//   - The authenticated customer
	//   - An error if the authentication fails
	AuthenticateCustomer(ctx context.Context, tenantID, email, password string) (*Customer, error)
}

type OrderService interface {
	// CreateOrder creates a new order with the given request.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - req: The request to create an order
	// Returns:
	//   - The created order
	//   - An error if the creation fails
	CreateOrder(ctx context.Context, tenantID string, req CreateOrderRequest) (*Order, error)

	// GetOrder retrieves an order by its ID and tenant ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - id: The ID of the order
	// Returns:
	//   - The retrieved order
	//   - An error if the retrieval fails
	GetOrder(ctx context.Context, tenantID, id string) (*Order, error)

	// UpdateOrderStatus updates the status of an order.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - id: The ID of the order
	//   - status: The new status of the order
	// Returns:
	//   - An error if the update fails
	UpdateOrderStatus(ctx context.Context, tenantID, id string, status OrderStatus) error

	// ListOrders lists all orders for a tenant with optional filters.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - filters: Optional filters for the orders
	// Returns:
	//   - A list of orders
	//   - An error if the retrieval fails
	ListOrders(ctx context.Context, tenantID string, filters OrderFilters) ([]Order, error)

	// GetCustomerOrders retrieves all orders for a customer by their ID and tenant ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - customerID: The ID of the customer
	// Returns:
	//   - A list of orders for the customer
	//   - An error if the retrieval fails
	GetCustomerOrders(ctx context.Context, tenantID, customerID string) ([]Order, error)

	// CancelOrder cancels an order by its ID and tenant ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - id: The ID of the order
	// Returns:
	//   - An error if the cancellation fails
	CancelOrder(ctx context.Context, tenantID, id string) error

	// RefundOrder refunds an order by its ID and tenant ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - id: The ID of the order
	//   - amount: The amount to refund
	// Returns:
	//   - An error if the refund fails
	RefundOrder(ctx context.Context, tenantID, id string, amount Money) error
}

type CartService interface {
	// GetCart retrieves a shopping cart by its ID and tenant ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - customerID: The ID of the customer
	// Returns:
	//   - The retrieved shopping cart
	//   - An error if the retrieval fails
	GetCart(ctx context.Context, tenantID, customerID string) (*ShoppingCart, error)

	// AddToCart adds an item to a shopping cart.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - customerID: The ID of the customer
	//   - req: The request to add an item to the cart
	// Returns:
	//   - The updated shopping cart
	//   - An error if the addition fails
	AddToCart(ctx context.Context, tenantID, customerID string, req AddToCartRequest) (*ShoppingCart, error)

	// UpdateCartItem updates an item in a shopping cart.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - cartID: The ID of the cart
	//   - itemID: The ID of the item
	//   - req: The request to update the item
	// Returns:
	//   - The updated shopping cart
	//   - An error if the update fails
	UpdateCartItem(ctx context.Context, tenantID, cartID, itemID string, req UpdateCartItemRequest) (*ShoppingCart, error)

	// RemoveFromCart removes an item from a shopping cart.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - cartID: The ID of the cart
	//   - itemID: The ID of the item
	// Returns:
	//   - The updated shopping cart
	//   - An error if the removal fails
	RemoveFromCart(ctx context.Context, tenantID, cartID, itemID string) (*ShoppingCart, error)

	// ClearCart clears all items from a shopping cart.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - cartID: The ID of the cart
	// Returns:
	//   - An error if the clearing fails
	ClearCart(ctx context.Context, tenantID, cartID string) error

	// ConvertCartToOrder converts a shopping cart to an order.
	// Parameters:
	//   - ctx: The context for the request
	//   - tenantID: The ID of the tenant
	//   - cartID: The ID of the cart
	//   - orderReq: The request to create an order
	// Returns:
	//   - The created order
	//   - An error if the conversion fails
	ConvertCartToOrder(ctx context.Context, tenantID, cartID string, orderReq CreateOrderRequest) (*Order, error)
}

// Filter types
// Memory optimized: 112 → 104 bytes (8 bytes saved)
type ProductFilters struct {
	PriceMin   *float64      `json:"priceMin"`
	PriceMax   *float64      `json:"priceMax"`
	InStock    *bool         `json:"inStock"`
	SortBy     string        `json:"sortBy"`
	SortOrder  string        `json:"sortOrder"`
	Status     ProductStatus `json:"status"`
	Categories []string      `json:"categories"`
	Tags       []string      `json:"tags"`
	Limit      int           `json:"limit"`
	Offset     int           `json:"offset"`
}

type OrderFilters struct {
	DateFrom   *time.Time  `json:"dateFrom"`
	DateTo     *time.Time  `json:"dateTo"`
	CustomerID string      `json:"customerId"`
	SortBy     string      `json:"sortBy"`
	SortOrder  string      `json:"sortOrder"`
	Status     OrderStatus `json:"status"`
	Limit      int         `json:"limit"`
	Offset     int         `json:"offset"`
}

// Utility functions
func generateID() string {
	return fmt.Sprintf("id_%d", time.Now().UnixNano())
}

func generateOrderNumber() string {
	return fmt.Sprintf("ORD-%d", time.Now().UnixNano()%10000000)
}

func calculateCartTotals(items []CartItem) CartTotals {
	var subtotal float64
	itemCount := 0

	for _, item := range items {
		subtotal += item.Total.Amount
		itemCount += item.Quantity
	}

	tax := subtotal * 0.08 // 8% tax rate
	shipping := 0.0
	if subtotal < 50 {
		shipping = 9.99 // Free shipping over $50
	}

	return CartTotals{
		Subtotal:  Money{Amount: subtotal, Currency: "USD"},
		Tax:       Money{Amount: tax, Currency: "USD"},
		Shipping:  Money{Amount: shipping, Currency: "USD"},
		Total:     Money{Amount: subtotal + tax + shipping, Currency: "USD"},
		ItemCount: itemCount,
	}
}

func calculateOrderTotals(items []OrderItem) OrderTotals {
	var subtotal float64

	for _, item := range items {
		subtotal += item.Total.Amount
	}

	tax := subtotal * 0.08 // 8% tax rate
	shipping := 0.0
	if subtotal < 50 {
		shipping = 9.99 // Free shipping over $50
	}

	return OrderTotals{
		Subtotal: Money{Amount: subtotal, Currency: "USD"},
		Tax:      Money{Amount: tax, Currency: "USD"},
		Shipping: Money{Amount: shipping, Currency: "USD"},
		Discount: Money{Amount: 0, Currency: "USD"},
		Total:    Money{Amount: subtotal + tax + shipping, Currency: "USD"},
		TaxRate:  0.08,
	}
}

// Tenant isolation middleware
func tenantIsolationMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Extract tenant ID from subdomain or header
			tenantID := ctx.Request.GetHeader("X-Tenant-ID")
			if tenantID == "" {
				// Try to extract from subdomain
				if host := ctx.Request.GetHeader("Host"); host != "" {
					if strings.Contains(host, ".") {
						parts := strings.Split(host, ".")
						if len(parts) > 2 {
							tenantID = parts[0]
						}
					}
				}
			}

			if tenantID == "" {
				return ctx.BadRequest("Tenant ID is required", nil)
			}

			// Add tenant ID to context
			ctx.Set("tenantID", tenantID)

			return next.Handle(ctx)
		})
	}
}

// Get tenant ID from context
func getTenantID(ctx *lift.Context) string {
	if tenantID, ok := ctx.Get("tenantID").(string); ok {
		return tenantID
	}
	return ""
}

func main() {
	// Create Lift application
	app := lift.New()

	// Enterprise e-commerce middleware stack
	// Note: Using basic middleware for now - full middleware integration pending
	// app.Use(middleware.Logger())
	// app.Use(middleware.Recover())
	// app.Use(middleware.CORS([]string{"*"}))

	// Enhanced observability for e-commerce
	// app.Use(middleware.EnhancedObservabilityMiddleware(middleware.EnhancedObservabilityConfig{
	// 	EnableMetrics: true,
	// 	EnableTracing: true,
	// 	EnableLogging: true,
	// }))

	// Security and rate limiting
	// app.Use(middleware.RateLimitMiddleware(middleware.RateLimitConfig{
	// 	DefaultLimit:  300, // Higher limit for e-commerce
	// 	DefaultWindow: time.Minute,
	// }))

	// Tenant isolation middleware
	app.Use(tenantIsolationMiddleware())

	// Setup all API routes
	if err := setupAPIRoutes(app); err != nil {
		log.Fatalf("Failed to setup API routes: %v", err)
	}

	log.Println("Starting Enterprise E-commerce Platform on port 8080...")
	log.Println("Multi-Tenant E-commerce Features:")
	log.Println("  ✓ Multi-tenant architecture with data isolation")
	log.Println("  ✓ Product catalog management")
	log.Println("  ✓ Order processing and payment integration")
	log.Println("  ✓ Customer management and authentication")
	log.Println("  ✓ Shopping cart and checkout workflows")
	log.Println("  ✓ Inventory management with real-time updates")
	log.Println("")
	log.Println("Available endpoints:")
	log.Println("  GET  /api/v1/health")
	log.Println("  POST /api/v1/tenants")
	log.Println("  GET  /api/v1/tenants")
	log.Println("  GET  /api/v1/tenants/:id")
	log.Println("  POST /api/v1/products")
	log.Println("  GET  /api/v1/products")
	log.Println("  GET  /api/v1/products/search")
	log.Println("  GET  /api/v1/products/:id")
	log.Println("  PUT  /api/v1/products/:id/inventory")
	log.Println("  POST /api/v1/customers")
	log.Println("  GET  /api/v1/customers")
	log.Println("  GET  /api/v1/customers/:id")
	log.Println("  POST /api/v1/customers/auth")
	log.Println("  GET  /api/v1/customers/:id/orders")
	log.Println("  POST /api/v1/orders")
	log.Println("  GET  /api/v1/orders")
	log.Println("  GET  /api/v1/orders/:id")
	log.Println("  PUT  /api/v1/orders/:id/status")
	log.Println("  GET  /api/v1/cart")
	log.Println("  POST /api/v1/cart/items")
	log.Println("  PUT  /api/v1/cart/:cartId/items/:itemId")
	log.Println("  DELETE /api/v1/cart/:cartId/items/:itemId")
	log.Println("  POST /api/v1/cart/:cartId/checkout")

	if err := app.Start(); err != nil {
		log.Fatalf("Failed to start app: %v", err)
	}
}

// setupAPIRoutes configures all the API routes for the e-commerce platform
func setupAPIRoutes(app *lift.App) error {
	type route struct {
		handler func(*lift.Context) error
		method  string
		path    string
	}

	routes := []route{
		// Health check
		{method: "GET", path: "/api/v1/health", handler: healthCheck},

		// Tenant management
		{method: "POST", path: "/api/v1/tenants", handler: createTenant},
		{method: "GET", path: "/api/v1/tenants", handler: listTenants},
		{method: "GET", path: "/api/v1/tenants/:id", handler: getTenant},

		// Products
		{method: "POST", path: "/api/v1/products", handler: createProduct},
		{method: "GET", path: "/api/v1/products", handler: listProducts},
		{method: "GET", path: "/api/v1/products/search", handler: searchProducts},
		{method: "GET", path: "/api/v1/products/:id", handler: getProduct},
		{method: "PUT", path: "/api/v1/products/:id/inventory", handler: updateProductInventory},

		// Customers
		{method: "POST", path: "/api/v1/customers", handler: createCustomer},
		{method: "GET", path: "/api/v1/customers", handler: listCustomers},
		{method: "GET", path: "/api/v1/customers/:id", handler: getCustomer},
		{method: "POST", path: "/api/v1/customers/auth", handler: authenticateCustomer},
		{method: "GET", path: "/api/v1/customers/:id/orders", handler: getCustomerOrders},

		// Orders
		{method: "POST", path: "/api/v1/orders", handler: createOrder},
		{method: "GET", path: "/api/v1/orders", handler: listOrders},
		{method: "GET", path: "/api/v1/orders/:id", handler: getOrder},
		{method: "PUT", path: "/api/v1/orders/:id/status", handler: updateOrderStatus},

		// Cart
		{method: "GET", path: "/api/v1/cart", handler: getCart},
		{method: "POST", path: "/api/v1/cart/items", handler: addToCart},
		{method: "PUT", path: "/api/v1/cart/:cartId/items/:itemId", handler: updateCartItem},
		{method: "DELETE", path: "/api/v1/cart/:cartId/items/:itemId", handler: removeFromCart},
		{method: "POST", path: "/api/v1/cart/:cartId/checkout", handler: checkout},
	}

	for _, r := range routes {
		var err error
		switch r.method {
		case "GET":
			err = app.GET(r.path, r.handler)
		case "POST":
			err = app.POST(r.path, r.handler)
		case "PUT":
			err = app.PUT(r.path, r.handler)
		case "DELETE":
			err = app.DELETE(r.path, r.handler)
		default:
			err = fmt.Errorf("unsupported method: %s", r.method)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func healthCheck(ctx *lift.Context) error {
	health := map[string]any{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "1.0.0",
		"platform":  "multi-tenant-ecommerce",
		"services": map[string]string{
			"database":          "healthy",
			"payment_gateway":   "healthy",
			"inventory_service": "healthy",
			"search_engine":     "healthy",
			"cache":             "healthy",
		},
		"features": map[string]any{
			"multi_tenant":        true,
			"real_time_inventory": true,
			"advanced_search":     true,
			"payment_processing":  true,
			"order_management":    true,
		},
		"metrics": map[string]any{
			"uptime_seconds":  3600,
			"total_tenants":   150,
			"total_products":  50000,
			"total_orders":    25000,
			"total_customers": 10000,
			"orders_today":    500,
		},
	}

	return ctx.OK(health)
}
