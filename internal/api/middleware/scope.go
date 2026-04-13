package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

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
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "authentication required",
			})
			return
		}

		jwtBID, ok := ctxBID.(uuid.UUID)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "invalid business_id claim",
			})
			return
		}

		if paramID := c.Param("business_id"); paramID != "" {
			paramBID, err := uuid.Parse(paramID)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": "invalid business_id parameter",
				})
				return
			}

			if paramBID != jwtBID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"success": false,
					"message": "access denied: resource belongs to a different business",
				})
				return
			}
		}

		c.Set(ContextKeyBusinessID, jwtBID)
		c.Next()
	}
}

// SubscriptionGuard gates a route behind a subscription feature.
func SubscriptionGuard(db *gorm.DB, feature string) gin.HandlerFunc {
	return func(c *gin.Context) {
		isPlatform, _ := c.Get(ContextKeyIsPlatform)
		if isPlatform == true {
			c.Next()
			return
		}

		businessID, exists := c.Get(ContextKeyBusinessID)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "authentication required",
			})
			return
		}

		var sub struct {
			Plan   string `gorm:"column:plan"`
			Status string `gorm:"column:status"`
		}

		if err := db.Table("subscriptions").
			Where("business_id = ?", businessID).
			Select("plan, status").
			First(&sub).Error; err != nil {
			sub.Plan = "free"
			sub.Status = "active"
		}

		if sub.Plan == "" {
			sub.Plan = "free"
		}
		if sub.Status == "" {
			sub.Status = "active"
		}

		if sub.Status != "active" && sub.Status != "grace_period" {
			c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
				"success":          false,
				"upgrade_required": true,
				"current_plan":     sub.Plan,
				"message":          "your subscription has expired. Please renew to continue.",
			})
			return
		}

		if !planHasFeature(sub.Plan, feature) {
			reqPlan := requiredPlanForFeature(feature)
			c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
				"success":          false,
				"upgrade_required": true,
				"feature":          feature,
				"current_plan":     sub.Plan,
				"required_plan":    reqPlan,
				"message":          featureGateMessage(feature, reqPlan),
			})
			return
		}

		c.Next()
	}
}

// LimitGuard checks resource limits before allowing a CREATE.
// For free plan sales/products, limits are monthly.
// For other limited resources, current all-time logic is used.
func LimitGuard(db *gorm.DB, resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		isPlatform, _ := c.Get(ContextKeyIsPlatform)
		if isPlatform == true {
			c.Next()
			return
		}

		businessID, exists := c.Get(ContextKeyBusinessID)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "authentication required",
			})
			return
		}

		var sub struct {
			Plan         string `gorm:"column:plan"`
			MaxStaff     int    `gorm:"column:max_staff"`
			MaxStores    int    `gorm:"column:max_stores"`
			MaxProducts  int    `gorm:"column:max_products"`
			MaxCustomers int    `gorm:"column:max_customers"`
		}

		if err := db.Table("subscriptions").
			Where("business_id = ?", businessID).
			Select("plan, max_staff, max_stores, max_products, max_customers").
			First(&sub).Error; err != nil {
			// Free plan defaults on lookup failure
			sub.Plan = "free"
			sub.MaxStaff = 0
			sub.MaxStores = 0
			sub.MaxProducts = 5
			sub.MaxCustomers = 0
		}

		if sub.Plan == "" {
			sub.Plan = "free"
		}

		tableMap := map[string]string{
			"sales":     "sales",
			"staff":     "business_users",
			"stores":    "stores",
			"products":  "products",
			"customers": "customers",
		}

		table, ok := tableMap[resource]
		if !ok {
			c.Next()
			return
		}

		// Free plan monthly limits
		freeMonthlyLimits := map[string]int{
			"sales":    5,
			"products": 5,
		}

		if sub.Plan == "free" && (resource == "sales" || resource == "products") {
			now := time.Now().UTC()
			startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
			resetsAt := startOfMonth.AddDate(0, 1, 0)

			var count int64
			query := db.Table(table).Where("business_id = ? AND created_at >= ?", businessID, startOfMonth)

			if resource == "products" {
				query = query.Where("is_active = true")
			}

			if err := query.Count(&count).Error; err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": "failed to validate plan limits",
				})
				return
			}

			limit := freeMonthlyLimits[resource]
			if int(count) >= limit {
				daysUntilReset := int(resetsAt.Sub(now).Hours() / 24)
				if daysUntilReset < 0 {
					daysUntilReset = 0
				}

				c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
					"success":          false,
					"upgrade_required": true,
					"current_count":    count,
					"limit":            limit,
					"resets_at":        resetsAt.Format(time.RFC3339),
					"days_until_reset": daysUntilReset,
					"message": fmt.Sprintf(
						"You have reached your %d %s limit for this month. Upgrade to Growth for unlimited %s, or wait %d days for your limit to reset.",
						limit,
						resource,
						resource,
						daysUntilReset,
					),
					"current_plan":  "free",
					"required_plan": "growth",
				})
				return
			}

			c.Next()
			return
		}

		limitMap := map[string]int{
			"staff":     sub.MaxStaff,
			"stores":    sub.MaxStores,
			"products":  sub.MaxProducts,
			"customers": sub.MaxCustomers,
		}

		maxAllowed, hasLimit := limitMap[resource]
		if !hasLimit {
			c.Next()
			return
		}

		// -1 means unlimited
		if maxAllowed == -1 {
			c.Next()
			return
		}

		var count int64
		query := db.Table(table).Where("business_id = ?", businessID)

		if resource == "staff" || resource == "products" || resource == "customers" {
			query = query.Where("is_active = true")
		}

		if err := query.Count(&count).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed to validate plan limits",
			})
			return
		}

		if int(count) >= maxAllowed {
			c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
				"success":          false,
				"upgrade_required": true,
				"current_count":    count,
				"limit":            maxAllowed,
				"current_plan":     sub.Plan,
				"required_plan":    requiredPlanForResource(resource),
				"message":          resourceLimitMessage(resource, maxAllowed),
			})
			return
		}

		c.Next()
	}
}

func planHasFeature(plan, feature string) bool {
	features := map[string][]string{
		"free": {
			"digital_store",
			"sales",
			"products",
		},
		"growth": {
			"digital_store",
			"sales",
			"products",
			"ledger",
			"invoices",
			"debt_tracking",
			"customers_basic",
			"staff_management",
		},
		"pro": {
			"digital_store",
			"sales",
			"products",
			"ledger",
			"invoices",
			"debt_tracking",
			"customers_basic",
			"staff_management",
			"recruitment",
			"stores",
			"identity_kyc",
			"public_profile",
			"expense_tracking",
			"custom_domain",
			"priority_support",
		},
		"custom": {
			"*",
		},
	}

	if plan == "custom" {
		return true
	}

	for _, f := range features[plan] {
		if f == "*" || f == feature {
			return true
		}
	}
	return false
}

func requiredPlanForFeature(feature string) string {
	proOnly := map[string]bool{
		"custom_domain":    true,
		"priority_support": true,
		"recruitment":      true,
		"stores":           true,
		"identity_kyc":     true,
		"public_profile":   true,
		"expense_tracking": true,
	}
	if proOnly[feature] {
		return "pro"
	}
	return "growth"
}

func requiredPlanForResource(resource string) string {
	proOnly := map[string]bool{
		"stores": true,
	}
	if proOnly[resource] {
		return "pro"
	}
	return "growth"
}

func featureGateMessage(feature, requiredPlan string) string {
	labels := map[string]string{
		"sales":            "Sales recording",
		"products":         "Product creation",
		"debt_tracking":    "Debt tracking",
		"expense_tracking": "Expense tracking",
		"recruitment":      "Recruitment and job postings",
		"identity_kyc":     "Business identity verification",
		"public_profile":   "Public business profile",
		"staff_management": "Staff management",
		"stores":           "Multiple store branches",
		"invoices":         "Invoice management",
		"custom_domain":    "Custom domain",
		"priority_support": "Priority support",
		"customers_basic":  "Customer management",
	}
	label := labels[feature]
	if label == "" {
		label = feature
	}
	return label + " is not available on your current plan. Upgrade to " + requiredPlan + " to unlock this feature."
}

func resourceLimitMessage(resource string, limit int) string {
	labels := map[string]string{
		"staff":     "staff accounts",
		"stores":    "store branches",
		"products":  "products",
		"customers": "customers",
	}
	label := labels[resource]
	if label == "" {
		label = resource
	}
	return "You have reached the limit of " + strconv.Itoa(limit) + " " + label + " on your current plan. Upgrade to add more."
}
