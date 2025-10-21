package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// Initialize services
var (
	tenantService   = &mockTenantService{}
	productService  = &mockProductService{}
	customerService = &mockCustomerService{}
	orderService    = &mockOrderService{}
	cartService     = &mockCartService{}
)

// Helper function for update operations to reduce code duplication
func handleUpdateOperation[T any](ctx *lift.Context,
	idParamName string,
	responseName string,
	performUpdate func(tenantID, resourceID string, req T) error,
	auditMessage string,
	additionalResponse func(T) map[string]any) error {
	tenantID := getTenantID(ctx)
	resourceID := ctx.PathParam(idParamName)

	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}
	if resourceID == "" {
		return ctx.BadRequest(strings.ToUpper(string(idParamName[0]))+idParamName[1:]+" ID is required", nil)
	}

	var req T
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.BadRequest("Invalid request", err)
	}

	err := performUpdate(tenantID, resourceID, req)
	if err != nil {
		return ctx.SystemError("Failed to perform update", err)
	}

	log.Printf(auditMessage, tenantID, resourceID, req)

	response := map[string]any{
		responseName: resourceID,
		"updated":    true,
		"timestamp":  time.Now(),
	}

	if additionalResponse != nil {
		for k, v := range additionalResponse(req) {
			response[k] = v
		}
	}

	return ctx.OK(response)
}

// Tenant handlers
func createTenant(ctx *lift.Context) error {
	var req CreateTenantRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request", 400)
	}

	tenant, err := tenantService.CreateTenant(ctx.Context, req)
	if err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to create tenant", 500)
	}

	log.Printf("ECOMMERCE AUDIT: Tenant created - ID: %s, Name: %s, Domain: %s",
		tenant.ID, tenant.Name, tenant.Domain)

	return ctx.Status(201).JSON(tenant)
}

func getTenant(ctx *lift.Context) error {
	tenantID := ctx.Param("id")
	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}

	tenant, err := tenantService.GetTenant(ctx.Context, tenantID)
	if err != nil {
		return lift.NotFound("Tenant not found")
	}

	return ctx.JSON(tenant)
}

func listTenants(ctx *lift.Context) error {
	limitStr := ctx.Query("limit")
	offsetStr := ctx.Query("offset")

	limit := 20
	offset := 0

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	tenants, err := tenantService.ListTenants(ctx.Context, limit, offset)
	if err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to list tenants", 500)
	}

	return ctx.JSON(map[string]any{
		"tenants": tenants,
		"count":   len(tenants),
		"limit":   limit,
		"offset":  offset,
	})
}

// Product handlers
func createProduct(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}

	var req CreateProductRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.BadRequest("Invalid request", err)
	}

	product, err := productService.CreateProduct(ctx.Request.Context(), tenantID, req)
	if err != nil {
		return ctx.SystemError("Failed to create product", err)
	}

	log.Printf("ECOMMERCE AUDIT: Product created - Tenant: %s, ID: %s, SKU: %s, Name: %s",
		tenantID, product.ID, product.SKU, product.Name)

	return ctx.Created(product)
}

func getProduct(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	productID := ctx.PathParam("id")

	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}
	if productID == "" {
		return ctx.BadRequest("Product ID is required", nil)
	}

	product, err := productService.GetProduct(ctx.Request.Context(), tenantID, productID)
	if err != nil {
		return ctx.NotFound("Product not found", err)
	}

	return ctx.OK(product)
}

func listProducts(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}

	// Parse filters from query parameters
	filters := ProductFilters{
		Limit:  20,
		Offset: 0,
	}

	if limitStr := ctx.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			filters.Limit = l
		}
	}

	if offsetStr := ctx.QueryParam("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			filters.Offset = o
		}
	}

	if categories := ctx.QueryParam("categories"); categories != "" {
		filters.Categories = []string{categories}
	}

	if status := ctx.QueryParam("status"); status != "" {
		filters.Status = ProductStatus(status)
	}

	products, err := productService.ListProducts(ctx.Request.Context(), tenantID, filters)
	if err != nil {
		return ctx.SystemError("Failed to list products", err)
	}

	return ctx.OK(map[string]any{
		"products": products,
		"count":    len(products),
		"filters":  filters,
	})
}

func searchProducts(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	query := ctx.QueryParam("q")

	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}
	if query == "" {
		return ctx.BadRequest("Search query is required", nil)
	}

	filters := ProductFilters{
		Limit:  20,
		Offset: 0,
	}

	products, err := productService.SearchProducts(ctx.Request.Context(), tenantID, query, filters)
	if err != nil {
		return ctx.SystemError("Product search failed", err)
	}

	return ctx.OK(map[string]any{
		"query":       query,
		"products":    products,
		"count":       len(products),
		"searched_at": time.Now(),
	})
}

func updateProductInventory(ctx *lift.Context) error {
	type inventoryUpdate struct {
		Quantity int `json:"quantity" validate:"required,min=0"`
	}

	return handleUpdateOperation(ctx, "id", "productId",
		func(tenantID, productID string, req inventoryUpdate) error {
			return productService.UpdateInventory(ctx.Request.Context(), tenantID, productID, req.Quantity)
		},
		"ECOMMERCE AUDIT: Inventory updated - Tenant: %s, Product: %s, Quantity: %d",
		func(req inventoryUpdate) map[string]any {
			return map[string]any{"quantity": req.Quantity}
		},
	)
}

// Customer handlers
func createCustomer(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}

	var req CreateCustomerRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.BadRequest("Invalid request", err)
	}

	customer, err := customerService.CreateCustomer(ctx.Request.Context(), tenantID, req)
	if err != nil {
		return ctx.SystemError("Failed to create customer", err)
	}

	log.Printf("ECOMMERCE AUDIT: Customer created - Tenant: %s, ID: %s, Email: %s",
		tenantID, customer.ID, customer.Email)

	return ctx.Created(customer)
}

func getCustomer(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	customerID := ctx.PathParam("id")

	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}
	if customerID == "" {
		return ctx.BadRequest("Customer ID is required", nil)
	}

	customer, err := customerService.GetCustomer(ctx.Request.Context(), tenantID, customerID)
	if err != nil {
		return ctx.NotFound("Customer not found", err)
	}

	return ctx.OK(customer)
}

func listCustomers(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}

	limitStr := ctx.QueryParam("limit")
	offsetStr := ctx.QueryParam("offset")

	limit := 20
	offset := 0

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	customers, err := customerService.ListCustomers(ctx.Request.Context(), tenantID, limit, offset)
	if err != nil {
		return ctx.SystemError("Failed to list customers", err)
	}

	return ctx.OK(map[string]any{
		"customers": customers,
		"count":     len(customers),
		"limit":     limit,
		"offset":    offset,
	})
}

func authenticateCustomer(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}

	var req struct {
		Email    string `json:"email" validate:"required,email"`
		Password string `json:"password" validate:"required"`
	}

	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.BadRequest("Invalid request", err)
	}

	customer, err := customerService.AuthenticateCustomer(ctx.Request.Context(), tenantID, req.Email, req.Password)
	if err != nil {
		return ctx.Unauthorized("Invalid credentials", err)
	}

	// Issue a signed JWT token (HS256) for demo purposes
	token, err := generateDemoJWT(map[string]any{
		"sub":   customer.ID,
		"ten":   tenantID,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
		"aud":   "ecommerce-demo",
		"iss":   "lift-demo",
		"email": customer.Email,
	})
	if err != nil {
		return ctx.SystemError("Failed to generate token", err)
	}

	log.Printf("ECOMMERCE AUDIT: Customer authenticated - Tenant: %s, Customer: %s, Email: %s",
		tenantID, customer.ID, customer.Email)

	return ctx.OK(map[string]any{
		"customer":         customer,
		"token":            token,
		"authenticated_at": time.Now(),
	})
}

// generateDemoJWT creates a minimal HS256 JWT without external deps.
// In production use a battle-tested JWT library and managed secrets.
func generateDemoJWT(claims map[string]any) (string, error) {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	b64 := func(b []byte) string {
		return base64.RawURLEncoding.EncodeToString(b)
	}

	signingInput := b64(headerJSON) + "." + b64(payloadJSON)
	secret := []byte(getJWTSecret())
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(signingInput))
	sig := mac.Sum(nil)

	return signingInput + "." + b64(sig), nil
}

func getJWTSecret() string {
	if v := os.Getenv("JWT_SECRET"); v != "" {
		return v
	}
	// Development-only default; override via env in real deployments
	return "dev-demo-secret-change-me"
}

// Order handlers
func createOrder(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}

	var req CreateOrderRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.BadRequest("Invalid request", err)
	}

	order, err := orderService.CreateOrder(ctx.Request.Context(), tenantID, req)
	if err != nil {
		return ctx.SystemError("Failed to create order", err)
	}

	log.Printf("ECOMMERCE AUDIT: Order created - Tenant: %s, ID: %s, Number: %s, Customer: %s, Total: %.2f",
		tenantID, order.ID, order.OrderNumber, order.CustomerID, order.Totals.Total.Amount)

	return ctx.Created(order)
}

func getOrder(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	orderID := ctx.PathParam("id")

	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}
	if orderID == "" {
		return ctx.BadRequest("Order ID is required", nil)
	}

	order, err := orderService.GetOrder(ctx.Request.Context(), tenantID, orderID)
	if err != nil {
		return ctx.NotFound("Order not found", err)
	}

	return ctx.OK(order)
}

func listOrders(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}

	filters := OrderFilters{
		Limit:  20,
		Offset: 0,
	}

	if limitStr := ctx.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			filters.Limit = l
		}
	}

	if offsetStr := ctx.QueryParam("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			filters.Offset = o
		}
	}

	if status := ctx.QueryParam("status"); status != "" {
		filters.Status = OrderStatus(status)
	}

	if customerID := ctx.QueryParam("customer_id"); customerID != "" {
		filters.CustomerID = customerID
	}

	orders, err := orderService.ListOrders(ctx.Request.Context(), tenantID, filters)
	if err != nil {
		return ctx.SystemError("Failed to list orders", err)
	}

	return ctx.OK(map[string]any{
		"orders":  orders,
		"count":   len(orders),
		"filters": filters,
	})
}

func updateOrderStatus(ctx *lift.Context) error {
	type statusUpdate struct {
		Status OrderStatus `json:"status" validate:"required"`
	}

	return handleUpdateOperation(ctx, "id", "orderId",
		func(tenantID, orderID string, req statusUpdate) error {
			return orderService.UpdateOrderStatus(ctx.Request.Context(), tenantID, orderID, req.Status)
		},
		"ECOMMERCE AUDIT: Order status updated - Tenant: %s, Order: %s, Status: %s",
		func(req statusUpdate) map[string]any {
			return map[string]any{"status": req.Status}
		},
	)
}

func getCustomerOrders(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	customerID := ctx.PathParam("id")

	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}
	if customerID == "" {
		return ctx.BadRequest("Customer ID is required", nil)
	}

	orders, err := orderService.GetCustomerOrders(ctx.Request.Context(), tenantID, customerID)
	if err != nil {
		return ctx.SystemError("Failed to get customer orders", err)
	}

	return ctx.OK(map[string]any{
		"customerId": customerID,
		"orders":     orders,
		"count":      len(orders),
	})
}

// Shopping cart handlers
func getCart(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	customerID := ctx.Request.GetHeader("X-Customer-ID")

	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}
	if customerID == "" {
		return ctx.BadRequest("Customer ID is required", nil)
	}

	cart, err := cartService.GetCart(ctx.Request.Context(), tenantID, customerID)
	if err != nil {
		return ctx.SystemError("Failed to get cart", err)
	}

	return ctx.OK(cart)
}

func addToCart(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	customerID := ctx.Request.GetHeader("X-Customer-ID")

	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}
	if customerID == "" {
		return ctx.BadRequest("Customer ID is required", nil)
	}

	var req AddToCartRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.BadRequest("Invalid request", err)
	}

	cart, err := cartService.AddToCart(ctx.Request.Context(), tenantID, customerID, req)
	if err != nil {
		return ctx.SystemError("Failed to add to cart", err)
	}

	log.Printf("ECOMMERCE AUDIT: Item added to cart - Tenant: %s, Customer: %s, Product: %s, Quantity: %d",
		tenantID, customerID, req.ProductID, req.Quantity)

	return ctx.OK(cart)
}

func updateCartItem(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	cartID := ctx.PathParam("cartId")
	itemID := ctx.PathParam("itemId")

	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}
	if cartID == "" {
		return ctx.BadRequest("Cart ID is required", nil)
	}
	if itemID == "" {
		return ctx.BadRequest("Item ID is required", nil)
	}

	var req UpdateCartItemRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.BadRequest("Invalid request", err)
	}

	cart, err := cartService.UpdateCartItem(ctx.Request.Context(), tenantID, cartID, itemID, req)
	if err != nil {
		return ctx.SystemError("Failed to update cart item", err)
	}

	return ctx.OK(cart)
}

func removeFromCart(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	cartID := ctx.PathParam("cartId")
	itemID := ctx.PathParam("itemId")

	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}
	if cartID == "" {
		return ctx.BadRequest("Cart ID is required", nil)
	}
	if itemID == "" {
		return ctx.BadRequest("Item ID is required", nil)
	}

	cart, err := cartService.RemoveFromCart(ctx.Request.Context(), tenantID, cartID, itemID)
	if err != nil {
		return ctx.SystemError("Failed to remove from cart", err)
	}

	return ctx.OK(cart)
}

func checkout(ctx *lift.Context) error {
	tenantID := getTenantID(ctx)
	cartID := ctx.PathParam("cartId")

	if tenantID == "" {
		return ctx.BadRequest("Tenant ID is required", nil)
	}
	if cartID == "" {
		return ctx.BadRequest("Cart ID is required", nil)
	}

	var req CreateOrderRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.BadRequest("Invalid request", err)
	}

	order, err := cartService.ConvertCartToOrder(ctx.Request.Context(), tenantID, cartID, req)
	if err != nil {
		return ctx.SystemError("Failed to checkout", err)
	}

	log.Printf("ECOMMERCE AUDIT: Checkout completed - Tenant: %s, Cart: %s, Order: %s, Total: %.2f",
		tenantID, cartID, order.ID, order.Totals.Total.Amount)

	return ctx.Created(order)
}
