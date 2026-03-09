// internal/api/middleware/scope.go
package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BusinessScope prevents cross-business data access.
// Validates that any :business_id URL param matches the JWT's business_id.
// Platform admins bypass this check entirely.
func BusinessScope(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		isPlatform, _ := c.Get(ContextKeyIsPlatform)
		if isPlatform == true {
			c.Next()
			return
		}
		ctxBID, exists := c.Get(ContextKeyBusinessID)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication required"})
			return
		}
		jwtBID, ok := ctxBID.(uuid.UUID)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"success": false, "message": "invalid business_id claim"})
			return
		}
		if paramID := c.Param("business_id"); paramID != "" {
			paramBID, err := uuid.Parse(paramID)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid business_id parameter"})
				return
			}
			if paramBID != jwtBID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "access denied: resource belongs to a different business"})
				return
			}
		}
		c.Set(ContextKeyBusinessID, jwtBID)
		c.Next()
	}
}

// SubscriptionGuard gates a route behind a subscription feature.
// Returns 402 with upgrade instructions if not on the right plan.
//
//	r.GET("/debts", middleware.SubscriptionGuard(db, "debt_tracking"), handler.List)
func SubscriptionGuard(db *gorm.DB, feature string) gin.HandlerFunc {
	return func(c *gin.Context) {
		isPlatform, _ := c.Get(ContextKeyIsPlatform)
		if isPlatform == true {
			c.Next()
			return
		}
		businessID, exists := c.Get(ContextKeyBusinessID)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication required"})
			return
		}
		var sub struct {
			Plan   string `gorm:"column:plan"`
			Status string `gorm:"column:status"`
		}
		if err := db.Table("subscriptions").Where("business_id = ?", businessID).
			Select("plan, status").First(&sub).Error; err != nil {
			sub.Plan, sub.Status = "free", "active"
		}
		if sub.Status != "active" && sub.Status != "grace_period" {
			c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
				"success": false, "upgrade_required": true, "current_plan": sub.Plan,
				"message": "your subscription has expired. Please renew to continue.",
			})
			return
		}
		if !planHasFeature(sub.Plan, feature) {
			req := requiredPlanForFeature(feature)
			c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
				"success": false, "upgrade_required": true, "feature": feature,
				"current_plan": sub.Plan, "required_plan": req,
				"message": featureGateMessage(feature, req),
			})
			return
		}
		c.Next()
	}
}

// LimitGuard checks countable resource limits before allowing a CREATE.
// Reads max_* values from the subscription so limits are dynamic per plan.
//
//	r.POST("/products", middleware.LimitGuard(db, "products"), handler.Create)
func LimitGuard(db *gorm.DB, resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		isPlatform, _ := c.Get(ContextKeyIsPlatform)
		if isPlatform == true {
			c.Next()
			return
		}
		businessID, exists := c.Get(ContextKeyBusinessID)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication required"})
			return
		}
		var sub struct {
			MaxStaff     int `gorm:"column:max_staff"`
			MaxStores    int `gorm:"column:max_stores"`
			MaxProducts  int `gorm:"column:max_products"`
			MaxCustomers int `gorm:"column:max_customers"`
		}
		if err := db.Table("subscriptions").Where("business_id = ?", businessID).
			Select("max_staff, max_stores, max_products, max_customers").First(&sub).Error; err != nil {
			sub.MaxStaff, sub.MaxStores, sub.MaxProducts, sub.MaxCustomers = 2, 1, 20, 50
		}
		tableMap := map[string]string{
			"staff": "business_users", "stores": "stores",
			"products": "products", "customers": "customers",
		}
		limitMap := map[string]int{
			"staff": sub.MaxStaff, "stores": sub.MaxStores,
			"products": sub.MaxProducts, "customers": sub.MaxCustomers,
		}
		table, ok := tableMap[resource]
		if !ok {
			c.Next()
			return
		}
		maxAllowed := limitMap[resource]
		if maxAllowed == -1 {
			c.Next()
			return
		}
		var count int64
		db.Table(table).Where("business_id = ? AND is_active = true", businessID).Count(&count)
		if int(count) >= maxAllowed {
			c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
				"success": false, "upgrade_required": true,
				"current_count": count, "limit": maxAllowed,
				"message": resourceLimitMessage(resource, maxAllowed),
			})
			return
		}
		c.Next()
	}
}

func planHasFeature(plan, feature string) bool {
	features := map[string][]string{
		"free":   {"sales", "ledger", "customers_basic", "digital_store"},
		"growth": {"sales", "ledger", "customers_basic", "digital_store", "debt_tracking", "expense_tracking", "recruitment", "identity_kyc", "public_profile", "staff_management", "stores", "invoices"},
		"pro":    {"sales", "ledger", "customers_basic", "digital_store", "debt_tracking", "expense_tracking", "recruitment", "identity_kyc", "public_profile", "staff_management", "stores", "invoices", "custom_domain", "priority_support"},
	}
	for _, f := range features[plan] {
		if f == feature {
			return true
		}
	}
	return false
}

func requiredPlanForFeature(feature string) string {
	if feature == "custom_domain" || feature == "priority_support" {
		return "pro"
	}
	return "growth"
}

func featureGateMessage(feature, requiredPlan string) string {
	labels := map[string]string{
		"debt_tracking": "Debt tracking", "expense_tracking": "Expense tracking",
		"recruitment": "Recruitment and job postings", "identity_kyc": "Business identity verification",
		"public_profile": "Public business profile", "staff_management": "Staff management",
		"stores": "Multiple store branches", "invoices": "Invoice management",
		"custom_domain": "Custom domain", "priority_support": "Priority support",
	}
	label := labels[feature]
	if label == "" {
		label = feature
	}
	return label + " is not available on your current plan. Upgrade to " + requiredPlan + " to unlock this feature."
}

func resourceLimitMessage(resource string, limit int) string {
	labels := map[string]string{
		"staff": "staff accounts", "stores": "store branches",
		"products": "products", "customers": "customers",
	}
	label := labels[resource]
	if label == "" {
		label = resource
	}
	return "You have reached the limit of " + strconv.Itoa(limit) + " " + label + " on your current plan. Upgrade to add more."
}
