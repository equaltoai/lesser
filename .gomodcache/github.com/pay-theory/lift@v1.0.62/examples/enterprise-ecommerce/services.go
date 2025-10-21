package main

import (
	"context"
	"strings"
	"time"
)

// Mock service implementations for the e-commerce platform

// Mock Tenant Service
type mockTenantService struct{}

func (m *mockTenantService) CreateTenant(_ context.Context, req CreateTenantRequest) (*Tenant, error) {
	tenant := &Tenant{
		ID:     generateID(),
		Name:   req.Name,
		Domain: req.Domain,
		Configuration: TenantConfig{
			Theme: ThemeConfig{
				PrimaryColor:   "#007bff",
				SecondaryColor: "#6c757d",
				Logo:           "/assets/logo.png",
				Favicon:        "/assets/favicon.ico",
			},
			PaymentMethods:  []string{"credit_card", "paypal", "stripe"},
			ShippingMethods: []string{"standard", "express", "overnight"},
			Currency:        "USD",
			Locale:          "en-US",
			Features: FeatureFlags{
				AdvancedSearch:    true,
				ProductReviews:    true,
				WishList:          true,
				Recommendations:   true,
				MultiCurrency:     false,
				InventoryTracking: true,
				Analytics:         true,
			},
			Limits: TenantLimits{
				MaxProducts:     10000,
				MaxOrders:       50000,
				MaxCustomers:    25000,
				StorageLimit:    5000, // 5GB
				BandwidthLimit:  100,  // 100GB
				APICallsPerHour: 10000,
			},
		},
		Subscription: Subscription{
			Plan:         req.Plan,
			Status:       "active",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0), // 1 year
			BillingCycle: "monthly",
			Amount:       Money{Amount: 99.99, Currency: "USD"},
		},
		Owner:     req.Owner,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		IsActive:  true,
	}

	return tenant, nil
}

func (m *mockTenantService) GetTenant(_ context.Context, id string) (*Tenant, error) {
	return &Tenant{
		ID:     id,
		Name:   "Demo Store",
		Domain: "demo.example.com",
		Configuration: TenantConfig{
			Theme: ThemeConfig{
				PrimaryColor:   "#007bff",
				SecondaryColor: "#6c757d",
				Logo:           "/assets/logo.png",
				Favicon:        "/assets/favicon.ico",
			},
			PaymentMethods:  []string{"credit_card", "paypal"},
			ShippingMethods: []string{"standard", "express"},
			Currency:        "USD",
			Locale:          "en-US",
			Features: FeatureFlags{
				AdvancedSearch:    true,
				ProductReviews:    true,
				WishList:          true,
				Recommendations:   true,
				InventoryTracking: true,
				Analytics:         true,
			},
		},
		Subscription: Subscription{
			Plan:   "professional",
			Status: "active",
		},
		Owner: TenantOwner{
			ID:    "owner_123",
			Name:  "John Smith",
			Email: "john@example.com",
		},
		CreatedAt: time.Now().Add(-180 * 24 * time.Hour),
		UpdatedAt: time.Now().Add(-24 * time.Hour),
		IsActive:  true,
	}, nil
}

func (m *mockTenantService) GetTenantByDomain(ctx context.Context, domain string) (*Tenant, error) {
	return m.GetTenant(ctx, "tenant_"+domain)
}

func (m *mockTenantService) UpdateTenant(_ context.Context, _ string, tenant *Tenant) error {
	tenant.UpdatedAt = time.Now()
	return nil
}

func (m *mockTenantService) ListTenants(_ context.Context, _, _ int) ([]Tenant, error) {
	tenants := []Tenant{
		{
			ID:     "tenant_1",
			Name:   "Fashion Store",
			Domain: "fashion.example.com",
			Subscription: Subscription{
				Plan:   "professional",
				Status: "active",
			},
			CreatedAt: time.Now().Add(-200 * 24 * time.Hour),
			IsActive:  true,
		},
		{
			ID:     "tenant_2",
			Name:   "Electronics Hub",
			Domain: "electronics.example.com",
			Subscription: Subscription{
				Plan:   "enterprise",
				Status: "active",
			},
			CreatedAt: time.Now().Add(-150 * 24 * time.Hour),
			IsActive:  true,
		},
	}

	return tenants, nil
}

func (m *mockTenantService) DeactivateTenant(_ context.Context, _ string) error {
	return nil
}

// Mock Product Service
type mockProductService struct{}

func (m *mockProductService) CreateProduct(_ context.Context, tenantID string, req CreateProductRequest) (*Product, error) {
	product := &Product{
		ID:          generateID(),
		TenantID:    tenantID,
		SKU:         req.SKU,
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		Inventory: Inventory{
			Quantity:          req.Inventory.Quantity,
			Reserved:          0,
			Available:         req.Inventory.Quantity,
			TrackStock:        req.Inventory.TrackStock,
			AllowBackorder:    req.Inventory.AllowBackorder,
			LowStockThreshold: req.Inventory.LowStockThreshold,
		},
		Categories: req.Categories,
		Tags:       req.Tags,
		Attributes: req.Attributes,
		Images:     req.Images,
		SEO:        req.SEO,
		Status:     ProductStatusActive,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	return product, nil
}

func (m *mockProductService) GetProduct(_ context.Context, tenantID, id string) (*Product, error) {
	return &Product{
		ID:           id,
		TenantID:     tenantID,
		SKU:          "DEMO-001",
		Name:         "Premium Wireless Headphones",
		Description:  "High-quality wireless headphones with noise cancellation and premium sound quality. Perfect for music lovers and professionals.",
		Price:        Money{Amount: 299.99, Currency: "USD"},
		ComparePrice: &Money{Amount: 399.99, Currency: "USD"},
		Inventory: Inventory{
			Quantity:          50,
			Reserved:          5,
			Available:         45,
			TrackStock:        true,
			AllowBackorder:    false,
			LowStockThreshold: 10,
		},
		Categories: []string{"Electronics", "Audio", "Headphones"},
		Tags:       []string{"wireless", "premium", "noise-canceling"},
		Attributes: map[string]any{
			"brand":        "AudioTech",
			"color":        "Black",
			"connectivity": "Bluetooth 5.0",
			"battery_life": "30 hours",
			"weight":       "250g",
		},
		Images: []ProductImage{
			{
				ID:        "img_1",
				URL:       "/images/headphones-1.jpg",
				AltText:   "Premium Wireless Headphones - Front View",
				Position:  1,
				IsPrimary: true,
			},
			{
				ID:       "img_2",
				URL:      "/images/headphones-2.jpg",
				AltText:  "Premium Wireless Headphones - Side View",
				Position: 2,
			},
		},
		SEO: SEOData{
			Title:       "Premium Wireless Headphones - AudioTech",
			Description: "Experience premium sound quality with our wireless headphones featuring noise cancellation and 30-hour battery life.",
			Keywords:    []string{"wireless headphones", "noise canceling", "premium audio", "bluetooth headphones"},
			Slug:        "premium-wireless-headphones",
		},
		Status:    ProductStatusActive,
		CreatedAt: time.Now().Add(-30 * 24 * time.Hour),
		UpdatedAt: time.Now().Add(-24 * time.Hour),
	}, nil
}

func (m *mockProductService) UpdateProduct(_ context.Context, _, _ string, product *Product) error {
	product.UpdatedAt = time.Now()
	return nil
}

func (m *mockProductService) DeleteProduct(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockProductService) ListProducts(_ context.Context, tenantID string, _ ProductFilters) ([]Product, error) {
	products := []Product{
		{
			ID:       "product_1",
			TenantID: tenantID,
			SKU:      "DEMO-001",
			Name:     "Premium Wireless Headphones",
			Price:    Money{Amount: 299.99, Currency: "USD"},
			Inventory: Inventory{
				Quantity:  50,
				Available: 45,
			},
			Categories: []string{"Electronics", "Audio"},
			Status:     ProductStatusActive,
			CreatedAt:  time.Now().Add(-30 * 24 * time.Hour),
		},
		{
			ID:       "product_2",
			TenantID: tenantID,
			SKU:      "DEMO-002",
			Name:     "Smart Fitness Watch",
			Price:    Money{Amount: 199.99, Currency: "USD"},
			Inventory: Inventory{
				Quantity:  25,
				Available: 20,
			},
			Categories: []string{"Electronics", "Wearables"},
			Status:     ProductStatusActive,
			CreatedAt:  time.Now().Add(-25 * 24 * time.Hour),
		},
		{
			ID:       "product_3",
			TenantID: tenantID,
			SKU:      "DEMO-003",
			Name:     "Organic Cotton T-Shirt",
			Price:    Money{Amount: 29.99, Currency: "USD"},
			Inventory: Inventory{
				Quantity:  100,
				Available: 95,
			},
			Categories: []string{"Clothing", "T-Shirts"},
			Status:     ProductStatusActive,
			CreatedAt:  time.Now().Add(-20 * 24 * time.Hour),
		},
	}

	return products, nil
}

func (m *mockProductService) SearchProducts(ctx context.Context, tenantID, query string, filters ProductFilters) ([]Product, error) {
	// Simulate search functionality
	allProducts, err := m.ListProducts(ctx, tenantID, filters)
	if err != nil {
		return nil, err
	}
	var results []Product

	query = strings.ToLower(query)
	for _, product := range allProducts {
		if strings.Contains(strings.ToLower(product.Name), query) ||
			strings.Contains(strings.ToLower(product.Description), query) ||
			strings.Contains(strings.ToLower(product.SKU), query) {
			results = append(results, product)
		}
	}

	return results, nil
}

func (m *mockProductService) UpdateInventory(_ context.Context, _, _ string, _ int) error {
	// Simulate inventory update
	return nil
}

// Mock Customer Service
type mockCustomerService struct{}

func (m *mockCustomerService) CreateCustomer(_ context.Context, tenantID string, req CreateCustomerRequest) (*Customer, error) {
	customer := &Customer{
		ID:             generateID(),
		TenantID:       tenantID,
		Email:          req.Email,
		Profile:        req.Profile,
		Addresses:      req.Addresses,
		PaymentMethods: []PaymentMethod{},
		OrderHistory:   []string{},
		Preferences:    req.Preferences,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		LastLoginAt:    time.Now(),
		IsActive:       true,
		Tags:           []string{"new_customer"},
	}

	return customer, nil
}

func (m *mockCustomerService) GetCustomer(_ context.Context, tenantID, id string) (*Customer, error) {
	return &Customer{
		ID:       id,
		TenantID: tenantID,
		Email:    "customer@example.com",
		Profile: CustomerProfile{
			FirstName: "Jane",
			LastName:  "Doe",
			Phone:     "+1-555-0123",
		},
		Addresses: []Address{
			{
				ID:         "addr_1",
				Type:       "shipping",
				FirstName:  "Jane",
				LastName:   "Doe",
				Address1:   "123 Main St",
				City:       "Anytown",
				State:      "CA",
				PostalCode: "12345",
				Country:    "US",
				IsDefault:  true,
			},
		},
		PaymentMethods: []PaymentMethod{
			{
				ID:        "pm_1",
				Type:      "card",
				Provider:  "visa",
				Last4:     "4242",
				IsDefault: true,
				CreatedAt: time.Now().Add(-30 * 24 * time.Hour),
			},
		},
		OrderHistory: []string{"order_1", "order_2"},
		Preferences: CustomerPreferences{
			Language:           "en",
			Currency:           "USD",
			EmailMarketing:     true,
			SMSMarketing:       false,
			PushNotifications:  true,
			FavoriteCategories: []string{"Electronics", "Clothing"},
		},
		CreatedAt:   time.Now().Add(-90 * 24 * time.Hour),
		UpdatedAt:   time.Now().Add(-24 * time.Hour),
		LastLoginAt: time.Now().Add(-2 * time.Hour),
		IsActive:    true,
		Tags:        []string{"vip_customer", "frequent_buyer"},
	}, nil
}

func (m *mockCustomerService) UpdateCustomer(_ context.Context, _, _ string, customer *Customer) error {
	customer.UpdatedAt = time.Now()
	return nil
}

func (m *mockCustomerService) ListCustomers(_ context.Context, tenantID string, _, _ int) ([]Customer, error) {
	customers := []Customer{
		{
			ID:       "customer_1",
			TenantID: tenantID,
			Email:    "john@example.com",
			Profile: CustomerProfile{
				FirstName: "John",
				LastName:  "Smith",
			},
			CreatedAt: time.Now().Add(-60 * 24 * time.Hour),
			IsActive:  true,
		},
		{
			ID:       "customer_2",
			TenantID: tenantID,
			Email:    "jane@example.com",
			Profile: CustomerProfile{
				FirstName: "Jane",
				LastName:  "Doe",
			},
			CreatedAt: time.Now().Add(-45 * 24 * time.Hour),
			IsActive:  true,
		},
	}

	return customers, nil
}

func (m *mockCustomerService) AuthenticateCustomer(ctx context.Context, tenantID, _, _ string) (*Customer, error) {
	// Simulate authentication
	return m.GetCustomer(ctx, tenantID, "customer_auth")
}

// Mock Order Service
type mockOrderService struct{}

func (m *mockOrderService) CreateOrder(_ context.Context, tenantID string, req CreateOrderRequest) (*Order, error) {
	totals := calculateOrderTotals(req.Items)

	order := &Order{
		ID:          generateID(),
		TenantID:    tenantID,
		CustomerID:  req.CustomerID,
		OrderNumber: generateOrderNumber(),
		Items:       req.Items,
		Totals:      totals,
		Payment:     req.Payment,
		Shipping:    req.Shipping,
		Status:      OrderStatusPending,
		Notes:       req.Notes,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	return order, nil
}

func (m *mockOrderService) GetOrder(_ context.Context, tenantID, id string) (*Order, error) {
	return &Order{
		ID:          id,
		TenantID:    tenantID,
		CustomerID:  "customer_123",
		OrderNumber: "ORD-1234567",
		Items: []OrderItem{
			{
				ID:        "item_1",
				ProductID: "product_1",
				SKU:       "DEMO-001",
				Name:      "Premium Wireless Headphones",
				Quantity:  1,
				Price:     Money{Amount: 299.99, Currency: "USD"},
				Total:     Money{Amount: 299.99, Currency: "USD"},
			},
		},
		Totals: OrderTotals{
			Subtotal: Money{Amount: 299.99, Currency: "USD"},
			Tax:      Money{Amount: 24.00, Currency: "USD"},
			Shipping: Money{Amount: 0.00, Currency: "USD"},
			Discount: Money{Amount: 0.00, Currency: "USD"},
			Total:    Money{Amount: 323.99, Currency: "USD"},
			TaxRate:  0.08,
		},
		Payment: PaymentInfo{
			Method:        "credit_card",
			Provider:      "stripe",
			TransactionID: "txn_123456789",
			Status:        "completed",
			Amount:        Money{Amount: 323.99, Currency: "USD"},
			ProcessedAt:   time.Now().Add(-1 * time.Hour),
		},
		Shipping: ShippingInfo{
			Method:   "standard",
			Provider: "fedex",
			Address: Address{
				FirstName:  "Jane",
				LastName:   "Doe",
				Address1:   "123 Main St",
				City:       "Anytown",
				State:      "CA",
				PostalCode: "12345",
				Country:    "US",
			},
			EstimatedDelivery: time.Now().Add(5 * 24 * time.Hour),
		},
		Status:    OrderStatusConfirmed,
		CreatedAt: time.Now().Add(-2 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}, nil
}

func (m *mockOrderService) UpdateOrderStatus(_ context.Context, _, _ string, _ OrderStatus) error {
	return nil
}

func (m *mockOrderService) ListOrders(_ context.Context, tenantID string, _ OrderFilters) ([]Order, error) {
	orders := []Order{
		{
			ID:          "order_1",
			TenantID:    tenantID,
			CustomerID:  "customer_1",
			OrderNumber: "ORD-1234567",
			Totals: OrderTotals{
				Total: Money{Amount: 323.99, Currency: "USD"},
			},
			Status:    OrderStatusConfirmed,
			CreatedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			ID:          "order_2",
			TenantID:    tenantID,
			CustomerID:  "customer_2",
			OrderNumber: "ORD-1234568",
			Totals: OrderTotals{
				Total: Money{Amount: 199.99, Currency: "USD"},
			},
			Status:    OrderStatusShipped,
			CreatedAt: time.Now().Add(-24 * time.Hour),
		},
	}

	return orders, nil
}

func (m *mockOrderService) GetCustomerOrders(ctx context.Context, tenantID, customerID string) ([]Order, error) {
	return m.ListOrders(ctx, tenantID, OrderFilters{CustomerID: customerID})
}

func (m *mockOrderService) CancelOrder(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockOrderService) RefundOrder(_ context.Context, _, _ string, _ Money) error {
	return nil
}

// Mock Cart Service
type mockCartService struct{}

func (m *mockCartService) GetCart(_ context.Context, tenantID, customerID string) (*ShoppingCart, error) {
	items := []CartItem{
		{
			ID:        "cart_item_1",
			ProductID: "product_1",
			Quantity:  1,
			Price:     Money{Amount: 299.99, Currency: "USD"},
			Total:     Money{Amount: 299.99, Currency: "USD"},
			AddedAt:   time.Now().Add(-30 * time.Minute),
		},
	}

	totals := calculateCartTotals(items)

	return &ShoppingCart{
		ID:         generateID(),
		TenantID:   tenantID,
		CustomerID: customerID,
		Items:      items,
		Totals:     totals,
		CreatedAt:  time.Now().Add(-30 * time.Minute),
		UpdatedAt:  time.Now().Add(-5 * time.Minute),
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}, nil
}

func (m *mockCartService) AddToCart(ctx context.Context, tenantID, customerID string, _ AddToCartRequest) (*ShoppingCart, error) {
	// Simulate adding item to cart
	return m.GetCart(ctx, tenantID, customerID)
}

func (m *mockCartService) UpdateCartItem(ctx context.Context, tenantID, _, _ string, _ UpdateCartItemRequest) (*ShoppingCart, error) {
	// Simulate updating cart item
	return m.GetCart(ctx, tenantID, "customer_from_cart")
}

func (m *mockCartService) RemoveFromCart(ctx context.Context, tenantID, _, _ string) (*ShoppingCart, error) {
	// Simulate removing item from cart
	return m.GetCart(ctx, tenantID, "customer_from_cart")
}

func (m *mockCartService) ClearCart(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockCartService) ConvertCartToOrder(ctx context.Context, tenantID, _ string, orderReq CreateOrderRequest) (*Order, error) {
	orderService := &mockOrderService{}
	return orderService.CreateOrder(ctx, tenantID, orderReq)
}
