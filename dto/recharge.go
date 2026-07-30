package dto

// ---------- 档位 ----------

type RechargeTierItem struct {
	Id            string   `json:"id"`
	Name          string   `json:"name"`
	Amount        int64    `json:"amount"`
	Price         float64  `json:"price"`
	OriginalPrice *float64 `json:"original_price,omitempty"`
	Bonus         int64    `json:"bonus"`
	IsRecommended bool     `json:"is_recommended"`
	SortOrder     int      `json:"sort_order"`
}

type GetRechargeTiersResponse struct {
	Tiers []RechargeTierItem `json:"tiers"`
}

// ---------- 订单 ----------

type CreateRechargeOrderRequest struct {
	TierId        string `json:"tier_id"`
	PaymentMethod string `json:"payment_method"` // "wechat" | "alipay"
}

type CreateRechargeOrderResponse struct {
	TradeNo    string  `json:"trade_no"`
	QrCodeData string  `json:"qr_code_data"` // 支付链接，前端渲染为二维码
	ExpireTime int64   `json:"expire_time"`  // Unix 秒
	Amount     float64 `json:"amount"`       // 实付金额（元）
	Points     int64   `json:"points"`       // 到账积分（含赠送）
}

// ---------- 订单状态 ----------

type GetOrderStatusResponse struct {
	TradeNo       string `json:"trade_no"`
	Status        string `json:"status"` // "pending" | "paid" | "expired" | "failed"
	CreditsAfter  *int64 `json:"credits_after,omitempty"`
}

// ---------- 协议 ----------

type GetRechargeAgreementResponse struct {
	Title   string `json:"title"`
	Content string `json:"content"` // Markdown
}

// ---------- 管理端档位 ----------

type AdminTierItem struct {
	Id            string   `json:"id"`
	Name          string   `json:"name"`
	Amount        int64    `json:"amount"`
	Price         float64  `json:"price"`
	OriginalPrice *float64 `json:"original_price,omitempty"`
	Bonus         int64    `json:"bonus"`
	IsRecommended bool     `json:"is_recommended"`
	IsActive      bool     `json:"is_active"`
	StartTime     *int64   `json:"start_time,omitempty"`
	EndTime       *int64   `json:"end_time,omitempty"`
	SortOrder     int      `json:"sort_order"`
	ChargeCount   int64    `json:"charge_count"`
	TotalRevenue  float64  `json:"total_revenue"`
	CreatedAt     int64    `json:"created_at"`
	UpdatedAt     int64    `json:"updated_at"`
}

type CreateTierRequest struct {
	Id            string   `json:"id"`
	Name          string   `json:"name"`
	Amount        int64    `json:"amount"`
	Price         float64  `json:"price"`
	OriginalPrice *float64 `json:"original_price,omitempty"`
	Bonus         int64    `json:"bonus"`
	IsRecommended bool     `json:"is_recommended"`
	IsActive      bool     `json:"is_active"`
	StartTime     *int64   `json:"start_time,omitempty"`
	EndTime       *int64   `json:"end_time,omitempty"`
	SortOrder     int      `json:"sort_order"`
}

type UpdateTierRequest struct {
	Name          *string  `json:"name,omitempty"`
	Amount        *int64   `json:"amount,omitempty"`
	Price         *float64 `json:"price,omitempty"`
	OriginalPrice *float64 `json:"original_price,omitempty"`
	Bonus         *int64   `json:"bonus,omitempty"`
	IsRecommended *bool    `json:"is_recommended,omitempty"`
	IsActive      *bool    `json:"is_active,omitempty"`
	StartTime     *int64   `json:"start_time,omitempty"`
	EndTime       *int64   `json:"end_time,omitempty"`
	SortOrder     *int     `json:"sort_order,omitempty"`
}
