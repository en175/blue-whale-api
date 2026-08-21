package material_library

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/material_setting"
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

// doArkAction 经当前配置的 provider 执行上游 Action：返回 2xx 响应体，非 2xx 返回解析出的 ArkError。
// 代理与管理端直连共用同一 provider 分发，保证鉴权/地址映射一致。
func doArkAction(ctx context.Context, action string, body []byte) ([]byte, error) {
	return doArkActionFor(ctx, material_setting.GetConfig().Provider, action, body)
}

// doArkActionFor 与 doArkAction 等价，但显式指定 provider（回调轮询用会话记录的 provider，
// 不受全局配置切换影响）。
func doArkActionFor(ctx context.Context, providerName, action string, body []byte) ([]byte, error) {
	resp, err := providerByName(providerName).DoAction(ctx, action, nil, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.Body, nil
	}
	return nil, arkErrorFrom(resp)
}

// arkErrorFrom 解析上游错误信封：tokenhub 用顶层 ResponseError，火山管控面用 ResponseMetadata.Error。
func arkErrorFrom(resp *Response) *ArkError {
	var envelope struct {
		ResponseError *ArkError `json:"ResponseError"`
		ResponseMetadata struct {
			Error *ArkError `json:"Error"`
		} `json:"ResponseMetadata"`
	}
	if err := common.Unmarshal(resp.Body, &envelope); err == nil {
		if envelope.ResponseError != nil {
			return envelope.ResponseError
		}
		if envelope.ResponseMetadata.Error != nil {
			return envelope.ResponseMetadata.Error
		}
	}
	return &ArkError{Code: "HTTP" + strconv.Itoa(resp.StatusCode), Message: "upstream error"}
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

// CreateVisualValidateSession 管理端直连：发起真人素材认证会话（上游 CreateVisualValidateSession），
// 返回 H5Link（给待验证者）与 BytedToken（会话标识，回调反查用）。
func CreateVisualValidateSession(ctx context.Context, callbackURL, projectName string) (bytedToken, h5Link string, err error) {
	body := map[string]any{"CallbackURL": callbackURL}
	if projectName != "" {
		body["ProjectName"] = projectName
	}
	payload, err := common.Marshal(body)
	if err != nil {
		return "", "", err
	}
	data, err := doArkAction(ctx, "CreateVisualValidateSession", payload)
	if err != nil {
		return "", "", err
	}
	var enveloped struct {
		Result struct {
			BytedToken string `json:"BytedToken"`
			H5Link     string `json:"H5Link"`
		} `json:"Result"`
	}
	if common.Unmarshal(data, &enveloped) == nil && enveloped.Result.BytedToken != "" {
		return enveloped.Result.BytedToken, enveloped.Result.H5Link, nil
	}
	// tokenhub 成功响应不包信封，直接返回顶层 BytedToken/H5Link。
	var direct struct {
		BytedToken string `json:"BytedToken"`
		H5Link     string `json:"H5Link"`
	}
	if err := common.Unmarshal(data, &direct); err != nil {
		return "", "", err
	}
	return direct.BytedToken, direct.H5Link, nil
}

// GetVisualValidateResult 用 BytedToken 查询真人认证结果（按会话记录的 provider 调用，
// 不受全局配置切换影响），返回真人素材组 id。H5 认证未完成时 GroupId 为空串（调用方继续轮询）。
// projectName 需与创建会话时一致（火山契约：设了 ProjectName 则查询必须传相同值，区分大小写），空则省略。
// 兼容两种响应格式：火山带 ResponseMetadata/Result 信封（GroupId 在 Result 内），
// tokenhub 直接返回 {"GroupId": "..."}（无信封）。
func GetVisualValidateResult(ctx context.Context, provider, bytedToken, projectName string) (string, error) {
	body := map[string]any{"BytedToken": bytedToken}
	if projectName != "" {
		body["ProjectName"] = projectName
	}
	payload, err := common.Marshal(body)
	if err != nil {
		return "", err
	}
	data, err := doArkActionFor(ctx, provider, "GetVisualValidateResult", payload)
	if err != nil {
		return "", err
	}
	var enveloped struct {
		Result struct {
			GroupId string `json:"GroupId"`
		} `json:"Result"`
	}
	if common.Unmarshal(data, &enveloped) == nil && enveloped.Result.GroupId != "" {
		return enveloped.Result.GroupId, nil
	}
	var direct struct {
		GroupId string `json:"GroupId"`
	}
	if common.Unmarshal(data, &direct) == nil {
		return direct.GroupId, nil
	}
	return "", nil // 认证未完成（两种格式下 GroupId 均为空）
}

// WaitVisualValidateGroupId 轮询 GetVisualValidateResult 直到拿到 GroupId 或超时。
// 上游 ValidatePending（认证未完成）或返回空 GroupId（组尚未就绪）时按 interval 重试；其余错误直接返回。
// 返回后的 GroupId 非空即真人素材组已就绪。
func WaitVisualValidateGroupId(ctx context.Context, provider, bytedToken, projectName string, timeout, interval time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		groupId, err := GetVisualValidateResult(ctx, provider, bytedToken, projectName)
		if err == nil && groupId != "" {
			return groupId, nil
		}
		var arkErr *ArkError
		if errors.As(err, &arkErr) && arkErr.Code == "ValidatePending" {
			// 认证未完成，继续等（token 有效期内）
		} else if err != nil {
			return "", err
		}
		// err==nil && groupId==""（200 但组未就绪）也继续等，直到超时。
		if time.Now().After(deadline) {
			return "", errors.New("visual validate timeout")
		}
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}
