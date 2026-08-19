package material_setting

import (
	"github.com/QuantumNous/new-api/setting/config"
)

// DefaultBaseURL 上游素材库（火山 ARK 兼容接口）默认地址，base_url 未配置时回退。
const DefaultBaseURL = "https://ai-tokenhub.ai"

// Config 素材库的内置凭证与上游地址。下游客户端不持有这些值，由网关注入。
// 租户隐含在 AK/SK 中，接口内不需要传租户 ID。
// 整段配置收敛在单一 DB 键 material_library.config 里，管理员以单个 JSON 编辑。
type Config struct {
	Enabled bool   `json:"enabled,omitempty"`
	AK      string `json:"ak,omitempty"`
	SK      string `json:"sk,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

// MaterialLibrary 是注册到全局配置系统的模块载体。外层包一层，使 configToMap 把
// Config 序列化成单个 JSON 值（DB 键 material_library.config），而非多个平铺键。
type MaterialLibrary struct {
	Config Config `json:"config"`
}

var materialLibrary = MaterialLibrary{}

func init() {
	config.GlobalConfig.Register("material_library", &materialLibrary)
}

// GetConfig 返回当前配置副本；base_url 为空时回退默认地址。
func GetConfig() Config {
	c := materialLibrary.Config
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	return c
}

// IsReady 表示素材库能力是否已启用且凭证齐全，是代理的前置守卫。
func (c Config) IsReady() bool {
	return c.Enabled && c.AK != "" && c.SK != ""
}
