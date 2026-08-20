package material_setting

import (
	"github.com/QuantumNous/new-api/setting/config"
)

// 素材库上游 Provider 枚举。
const (
	// ProviderTokenhub 对接 ai-tokenhub（火山 ARK 兼容接口）。
	ProviderTokenhub = "tokenhub"
	// ProviderVolcengine 对接火山引擎官方素材库（管控面 API，SigV4 签名）。
	ProviderVolcengine = "volcengine"
)

// DefaultBaseURL tokenhub 上游（火山 ARK 兼容接口）默认地址，base_url 未配置时回退。
const DefaultBaseURL = "https://ai-tokenhub.ai"

// DefaultVolcengineBaseURL 火山引擎素材库管控面默认地址（不同 Region 由 volcengine_base_url 配置覆盖）。
const DefaultVolcengineBaseURL = "https://ark-cn-beijing.volcengineapi.com"

// DefaultVolcengineRegion 火山管控面默认区域。
const DefaultVolcengineRegion = "cn-beijing"

// Config 素材库的内置凭证与上游地址。下游客户端不持有这些值，由网关注入。
// 租户隐含在凭证中，接口内不需要传租户 ID。
// Provider 选择上游（tokenhub 默认 / volcengine），各上游凭证字段独立，IsReady 按 Provider 校验。
// 整段配置收敛在单一 DB 键 material_library.config 里，管理员以单个 JSON 编辑。
type Config struct {
	Enabled  bool   `json:"enabled,omitempty"`
	Provider string `json:"provider,omitempty"`
	// tokenhub 凭证与地址。
	AK      string `json:"ak,omitempty"`
	SK      string `json:"sk,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	// 火山引擎凭证与地址（provider=volcengine 时生效）。
	VolcengineAccessKey string `json:"volcengine_access_key,omitempty"`
	VolcengineSecretKey string `json:"volcengine_secret_key,omitempty"`
	VolcengineRegion    string `json:"volcengine_region,omitempty"`
	VolcengineBaseURL   string `json:"volcengine_base_url,omitempty"`
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

// GetConfig 返回当前配置副本；provider 空回退 tokenhub，各上游地址空时回退默认值。
func GetConfig() Config {
	c := materialLibrary.Config
	if c.Provider == "" {
		c.Provider = ProviderTokenhub
	}
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	if c.VolcengineRegion == "" {
		c.VolcengineRegion = DefaultVolcengineRegion
	}
	if c.VolcengineBaseURL == "" {
		c.VolcengineBaseURL = DefaultVolcengineBaseURL
	}
	return c
}

// IsReady 表示素材库能力是否已启用且当前上游凭证齐全，是代理与管理界面的前置守卫。
// 按 Provider 校验对应凭证组：tokenhub 需要 ak/sk，volcengine 需要 access_key/secret_key。
func (c Config) IsReady() bool {
	if !c.Enabled {
		return false
	}
	switch c.Provider {
	case ProviderVolcengine:
		return c.VolcengineAccessKey != "" && c.VolcengineSecretKey != ""
	default:
		return c.AK != "" && c.SK != ""
	}
}
