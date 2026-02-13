package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
	"github.com/smallnest/goclaw/config"
)

const (
	defaultFeishuDrivePageSize = 20
	maxFeishuDrivePageSize     = 200
)

// FeishuDocsTool provides Feishu document and drive operations.
type FeishuDocsTool struct {
	client *lark.Client
}

// NewFeishuDocsTool creates a Feishu docs tool with explicit credentials.
func NewFeishuDocsTool(appID, appSecret string) *FeishuDocsTool {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	if appID == "" || appSecret == "" {
		return nil
	}

	return &FeishuDocsTool{
		client: lark.NewClient(appID, appSecret),
	}
}

// NewFeishuDocsToolFromConfig creates a Feishu docs tool from Feishu channel config.
func NewFeishuDocsToolFromConfig(cfg config.FeishuChannelConfig) *FeishuDocsTool {
	appID, appSecret := ResolveFeishuCredentials(cfg)
	return NewFeishuDocsTool(appID, appSecret)
}

// ResolveFeishuCredentials resolves Feishu app credentials from top-level or account configs.
func ResolveFeishuCredentials(cfg config.FeishuChannelConfig) (string, string) {
	if appID, appSecret, ok := normalizeFeishuCredentials(cfg.AppID, cfg.AppSecret); ok {
		return appID, appSecret
	}

	accountIDs := make([]string, 0, len(cfg.Accounts))
	for accountID := range cfg.Accounts {
		accountIDs = append(accountIDs, accountID)
	}
	sort.Strings(accountIDs)

	// Prefer enabled accounts first.
	for _, accountID := range accountIDs {
		account := cfg.Accounts[accountID]
		if !account.Enabled {
			continue
		}
		if appID, appSecret, ok := normalizeFeishuCredentials(account.AppID, account.AppSecret); ok {
			return appID, appSecret
		}
	}

	// Fallback to any account with valid credentials.
	for _, accountID := range accountIDs {
		account := cfg.Accounts[accountID]
		if appID, appSecret, ok := normalizeFeishuCredentials(account.AppID, account.AppSecret); ok {
			return appID, appSecret
		}
	}

	return "", ""
}

func normalizeFeishuCredentials(appID, appSecret string) (string, string, bool) {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	if appID == "" || appSecret == "" {
		return "", "", false
	}
	return appID, appSecret, true
}

// GetTools returns all Feishu document/drive tools.
func (t *FeishuDocsTool) GetTools() []Tool {
	if t == nil || t.client == nil {
		return nil
	}

	return []Tool{
		NewBaseTool(
			"feishu_doc_create",
			"Create a new Feishu document.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type":        "string",
						"description": "Document title.",
					},
					"folder_token": map[string]interface{}{
						"type":        "string",
						"description": "Optional parent folder token. Empty means root.",
					},
				},
				"required": []string{"title"},
			},
			t.createDocument,
		),
		NewBaseTool(
			"feishu_doc_read",
			"Read Feishu document raw text content.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"document_id": map[string]interface{}{
						"type":        "string",
						"description": "Feishu document ID.",
					},
					"lang": map[string]interface{}{
						"type":        "integer",
						"description": "Language for content. 0=zh, 1=en, 2=jp.",
						"default":     0,
					},
				},
				"required": []string{"document_id"},
			},
			t.readDocument,
		),
		NewBaseTool(
			"feishu_drive_list",
			"List files in a Feishu Drive folder.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"folder_token": map[string]interface{}{
						"type":        "string",
						"description": "Folder token. Empty means root.",
					},
					"page_size": map[string]interface{}{
						"type":        "integer",
						"description": "Page size (1-200).",
						"default":     defaultFeishuDrivePageSize,
					},
					"page_token": map[string]interface{}{
						"type":        "string",
						"description": "Next page token from previous response.",
					},
					"order_by": map[string]interface{}{
						"type":        "string",
						"description": "Sort field, e.g. EditedTime or CreatedTime.",
					},
					"direction": map[string]interface{}{
						"type":        "string",
						"description": "Sort direction: ASC or DESC.",
					},
				},
			},
			t.listDriveFiles,
		),
		NewBaseTool(
			"feishu_drive_create_folder",
			"Create a folder in Feishu Drive.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "New folder name.",
					},
					"folder_token": map[string]interface{}{
						"type":        "string",
						"description": "Optional parent folder token. Empty means root.",
					},
				},
				"required": []string{"name"},
			},
			t.createDriveFolder,
		),
	}
}

func (t *FeishuDocsTool) createDocument(ctx context.Context, params map[string]interface{}) (string, error) {
	title, err := getRequiredStringParam(params, "title")
	if err != nil {
		return "", err
	}
	folderToken, err := getOptionalStringParam(params, "folder_token")
	if err != nil {
		return "", err
	}

	bodyBuilder := larkdocx.NewCreateDocumentReqBodyBuilder().Title(title)
	if folderToken != "" {
		bodyBuilder.FolderToken(folderToken)
	}

	req := larkdocx.NewCreateDocumentReqBuilder().
		Body(bodyBuilder.Build()).
		Build()

	resp, err := t.client.Docx.V1.Document.Create(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create Feishu document: %w", err)
	}
	if !resp.Success() {
		return "", feishuAPIError(resp.Code, resp.Msg)
	}

	var documentID string
	documentTitle := title
	var revisionID interface{}
	revisionID = nil

	if resp.Data != nil && resp.Data.Document != nil {
		documentID = derefString(resp.Data.Document.DocumentId)
		if titleFromAPI := derefString(resp.Data.Document.Title); titleFromAPI != "" {
			documentTitle = titleFromAPI
		}
		if resp.Data.Document.RevisionId != nil {
			revisionID = *resp.Data.Document.RevisionId
		}
	}

	result := map[string]interface{}{
		"document_id": documentID,
		"title":       documentTitle,
		"revision_id": revisionID,
	}
	if folderToken != "" {
		result["folder_token"] = folderToken
	}
	return marshalToolResult(result)
}

func (t *FeishuDocsTool) readDocument(ctx context.Context, params map[string]interface{}) (string, error) {
	documentID, err := getRequiredStringParam(params, "document_id")
	if err != nil {
		return "", err
	}
	lang, err := getOptionalIntParam(params, "lang", larkdocx.LangZH)
	if err != nil {
		return "", err
	}

	rawReq := larkdocx.NewRawContentDocumentReqBuilder().
		DocumentId(documentID).
		Lang(lang).
		Build()
	rawResp, err := t.client.Docx.V1.Document.RawContent(ctx, rawReq)
	if err != nil {
		return "", fmt.Errorf("failed to read Feishu document content: %w", err)
	}
	if !rawResp.Success() {
		return "", feishuAPIError(rawResp.Code, rawResp.Msg)
	}

	result := map[string]interface{}{
		"document_id": documentID,
		"lang":        lang,
		"content":     "",
	}
	if rawResp.Data != nil {
		result["content"] = derefString(rawResp.Data.Content)
	}

	// Metadata lookup is best-effort and should not block content retrieval.
	getReq := larkdocx.NewGetDocumentReqBuilder().
		DocumentId(documentID).
		Build()
	getResp, getErr := t.client.Docx.V1.Document.Get(ctx, getReq)
	if getErr != nil {
		result["document_meta_error"] = getErr.Error()
		return marshalToolResult(result)
	}
	if !getResp.Success() {
		result["document_meta_error"] = feishuAPIError(getResp.Code, getResp.Msg).Error()
		return marshalToolResult(result)
	}
	if getResp.Data != nil && getResp.Data.Document != nil {
		result["title"] = derefString(getResp.Data.Document.Title)
		if getResp.Data.Document.RevisionId != nil {
			result["revision_id"] = *getResp.Data.Document.RevisionId
		}
	}

	return marshalToolResult(result)
}

func (t *FeishuDocsTool) listDriveFiles(ctx context.Context, params map[string]interface{}) (string, error) {
	folderToken, err := getOptionalStringParam(params, "folder_token")
	if err != nil {
		return "", err
	}
	pageToken, err := getOptionalStringParam(params, "page_token")
	if err != nil {
		return "", err
	}
	orderBy, err := getOptionalStringParam(params, "order_by")
	if err != nil {
		return "", err
	}
	direction, err := getOptionalStringParam(params, "direction")
	if err != nil {
		return "", err
	}
	pageSize, err := getOptionalIntParam(params, "page_size", defaultFeishuDrivePageSize)
	if err != nil {
		return "", err
	}

	if pageSize < 1 {
		pageSize = defaultFeishuDrivePageSize
	}
	if pageSize > maxFeishuDrivePageSize {
		pageSize = maxFeishuDrivePageSize
	}

	reqBuilder := larkdrive.NewListFileReqBuilder().PageSize(pageSize)
	if folderToken != "" {
		reqBuilder.FolderToken(folderToken)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}
	if orderBy != "" {
		reqBuilder.OrderBy(orderBy)
	}
	if direction != "" {
		reqBuilder.Direction(strings.ToUpper(direction))
	}

	resp, err := t.client.Drive.V1.File.List(ctx, reqBuilder.Build())
	if err != nil {
		return "", fmt.Errorf("failed to list Feishu drive files: %w", err)
	}
	if !resp.Success() {
		return "", feishuAPIError(resp.Code, resp.Msg)
	}

	files := make([]map[string]interface{}, 0)
	hasMore := false
	nextPageToken := ""

	if resp.Data != nil {
		hasMore = derefBool(resp.Data.HasMore)
		nextPageToken = derefString(resp.Data.NextPageToken)
		for _, file := range resp.Data.Files {
			if file == nil {
				continue
			}
			item := map[string]interface{}{
				"token":         derefString(file.Token),
				"name":          derefString(file.Name),
				"type":          derefString(file.Type),
				"parent_token":  derefString(file.ParentToken),
				"url":           derefString(file.Url),
				"owner_id":      derefString(file.OwnerId),
				"created_time":  derefString(file.CreatedTime),
				"modified_time": derefString(file.ModifiedTime),
			}
			if file.ShortcutInfo != nil {
				item["shortcut_target_type"] = derefString(file.ShortcutInfo.TargetType)
				item["shortcut_target_token"] = derefString(file.ShortcutInfo.TargetToken)
			}
			files = append(files, item)
		}
	}

	result := map[string]interface{}{
		"folder_token":    folderToken,
		"page_size":       pageSize,
		"next_page_token": nextPageToken,
		"has_more":        hasMore,
		"files":           files,
	}

	return marshalToolResult(result)
}

func (t *FeishuDocsTool) createDriveFolder(ctx context.Context, params map[string]interface{}) (string, error) {
	name, err := getRequiredStringParam(params, "name")
	if err != nil {
		return "", err
	}
	folderToken, err := getOptionalStringParam(params, "folder_token")
	if err != nil {
		return "", err
	}

	bodyBuilder := larkdrive.NewCreateFolderFileReqBodyBuilder().Name(name)
	if folderToken != "" {
		bodyBuilder.FolderToken(folderToken)
	}

	req := larkdrive.NewCreateFolderFileReqBuilder().
		Body(bodyBuilder.Build()).
		Build()
	resp, err := t.client.Drive.V1.File.CreateFolder(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create Feishu drive folder: %w", err)
	}
	if !resp.Success() {
		return "", feishuAPIError(resp.Code, resp.Msg)
	}

	result := map[string]interface{}{
		"name":                name,
		"parent_folder_token": folderToken,
		"created_folder_token": func() string {
			if resp.Data == nil {
				return ""
			}
			return derefString(resp.Data.Token)
		}(),
		"url": func() string {
			if resp.Data == nil {
				return ""
			}
			return derefString(resp.Data.Url)
		}(),
	}
	return marshalToolResult(result)
}

func getRequiredStringParam(params map[string]interface{}, key string) (string, error) {
	value, err := getOptionalStringParam(params, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s is required and must be a non-empty string", key)
	}
	return value, nil
}

func getOptionalStringParam(params map[string]interface{}, key string) (string, error) {
	value, exists := params[key]
	if !exists || value == nil {
		return "", nil
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return strings.TrimSpace(stringValue), nil
}

func getOptionalIntParam(params map[string]interface{}, key string, defaultValue int) (int, error) {
	value, exists := params[key]
	if !exists || value == nil {
		return defaultValue, nil
	}

	switch v := value.(type) {
	case int:
		return v, nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case float32:
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("%s must be a number", key)
	}
}

func feishuAPIError(code int, msg string) error {
	return fmt.Errorf("feishu api error: %d %s", code, msg)
}

func marshalToolResult(result map[string]interface{}) (string, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tool result: %w", err)
	}
	return string(data), nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefBool(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}
