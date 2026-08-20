package material_library

import (
	"context"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

// ArkAsset 上游素材对象（管理界面表格用到的字段）。
type ArkAsset struct {
	Id         string `json:"Id"`
	Name       string `json:"Name"`
	URL        string `json:"URL"`
	GroupId    string `json:"GroupId"`
	AssetType  string `json:"AssetType"`
	Status     string `json:"Status"`
	CreateTime string `json:"CreateTime"`
	UpdateTime string `json:"UpdateTime"`
}

// ListAssetsResult 对应上游 ListAssets 的 Result。
type ListAssetsResult struct {
	TotalCount int        `json:"TotalCount"`
	Items      []ArkAsset `json:"Items"`
	PageNumber int        `json:"PageNumber"`
	PageSize   int        `json:"PageSize"`
}

// ArkError 上游 ARK 接口的 ResponseError 信封。
type ArkError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

func (e *ArkError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

// doArkAction 经当前 provider 执行上游 Action：返回 2xx 响应体，非 2xx 返回解析出的 ArkError。
// 代理与管理端直连共用同一 provider 分发，保证鉴权/地址映射一致。
func doArkAction(ctx context.Context, action string, body []byte) ([]byte, error) {
	resp, err := currentProvider().DoAction(ctx, action, nil, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.Body, nil
	}
	// 错误信封兼容两种格式：tokenhub 用顶层 ResponseError，火山管控面用 ResponseMetadata.Error。
	var envelope struct {
		ResponseError *ArkError `json:"ResponseError"`
		ResponseMetadata struct {
			Error *ArkError `json:"Error"`
		} `json:"ResponseMetadata"`
	}
	if err := common.Unmarshal(resp.Body, &envelope); err == nil {
		if envelope.ResponseError != nil {
			return nil, envelope.ResponseError
		}
		if envelope.ResponseMetadata.Error != nil {
			return nil, envelope.ResponseMetadata.Error
		}
	}
	return nil, &ArkError{Code: "HTTP" + strconv.Itoa(resp.StatusCode), Message: "upstream error"}
}

// ListAssets 按素材组 id 列表查询上游素材，供管理界面右侧表格使用。
// groupType 非空时透传给上游 Filter.GroupType（火山管控面必填；tokenhub 兼容忽略），
// name 非空时透传做名称模糊匹配。
func ListAssets(ctx context.Context, groupIDs []string, groupType string, page, pageSize int, name string) (*ListAssetsResult, error) {
	filter := map[string]any{"GroupIds": groupIDs}
	if groupType != "" {
		filter["GroupType"] = groupType
	}
	if name != "" {
		filter["Name"] = name
	}
	body, err := common.Marshal(map[string]any{
		"Filter":     filter,
		"PageNumber": page,
		"PageSize":   pageSize,
	})
	if err != nil {
		return nil, err
	}
	data, err := doArkAction(ctx, "ListAssets", body)
	if err != nil {
		return nil, err
	}
	var result ListAssetsResult
	if err := common.Unmarshal(data, &struct {
		Result *ListAssetsResult `json:"Result"`
	}{Result: &result}); err != nil {
		return nil, err
	}
	return &result, nil
}

// resultID 解析 ARK 成功响应的 Result.Id（创建类接口返回上游 id）。
func resultID(data []byte) (string, error) {
	var out struct {
		Result struct {
			Id string `json:"Id"`
		} `json:"Result"`
	}
	if err := common.Unmarshal(data, &out); err != nil {
		return "", err
	}
	return out.Result.Id, nil
}

// CreateAssetGroup 创建素材组，返回上游组 id。
func CreateAssetGroup(ctx context.Context, name, description, groupType string) (string, error) {
	body, err := common.Marshal(map[string]any{
		"Name":        name,
		"Description": description,
		"GroupType":   groupType,
	})
	if err != nil {
		return "", err
	}
	data, err := doArkAction(ctx, "CreateAssetGroup", body)
	if err != nil {
		return "", err
	}
	return resultID(data)
}

// DeleteAssetGroup 删除素材组。
func DeleteAssetGroup(ctx context.Context, groupID string) error {
	body, err := common.Marshal(map[string]any{"Id": groupID})
	if err != nil {
		return err
	}
	_, err = doArkAction(ctx, "DeleteAssetGroup", body)
	return err
}

// CreateAsset 上传素材（URL 直传），返回上游素材 id。
func CreateAsset(ctx context.Context, groupID, url, name, assetType string) (string, error) {
	body, err := common.Marshal(map[string]any{
		"GroupId":   groupID,
		"URL":       url,
		"Name":      name,
		"AssetType": assetType,
	})
	if err != nil {
		return "", err
	}
	data, err := doArkAction(ctx, "CreateAsset", body)
	if err != nil {
		return "", err
	}
	return resultID(data)
}

// UpdateAsset 编辑素材（改名）。
func UpdateAsset(ctx context.Context, assetID, name string) error {
	body, err := common.Marshal(map[string]any{"Id": assetID, "Name": name})
	if err != nil {
		return err
	}
	_, err = doArkAction(ctx, "UpdateAsset", body)
	return err
}

// DeleteAsset 删除素材。
func DeleteAsset(ctx context.Context, assetID string) error {
	body, err := common.Marshal(map[string]any{"Id": assetID})
	if err != nil {
		return err
	}
	_, err = doArkAction(ctx, "DeleteAsset", body)
	return err
}
