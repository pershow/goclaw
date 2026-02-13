package gateway

import (
	"encoding/json"
	"fmt"
)

// ProtocolVersion 当前协议版本
const ProtocolVersion = "1.0"

// MessageType 消息类型
type MessageType string

const (
	// Request 请求
	MessageTypeRequest MessageType = "request"
	// Response 响应
	MessageTypeResponse MessageType = "response"
	// Notification 通知
	MessageTypeNotification MessageType = "notification"
)

// JSONRPCRequest JSON-RPC 请求
type JSONRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      string                 `json:"id,omitempty"` // 通知可以没有ID
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// JSONRPCResponse JSON-RPC 响应
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError RPC 错误
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// Error codes
const (
	ErrorParseError     = -32700
	ErrorInvalidRequest = -32600
	ErrorMethodNotFound = -32601
	ErrorInvalidParams  = -32602
	ErrorInternalError  = -32603
)

// NewErrorResponse 创建错误响应
func NewErrorResponse(id string, code int, message string) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
}

// NewSuccessResponse 创建成功响应
func NewSuccessResponse(id string, result interface{}) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// MethodRegistry 方法注册表
type MethodRegistry struct {
	methods map[string]MethodHandler
}

// MethodHandler 方法处理器
type MethodHandler func(sessionID string, params map[string]interface{}) (interface{}, error)

// NewMethodRegistry 创建方法注册表
func NewMethodRegistry() *MethodRegistry {
	return &MethodRegistry{
		methods: make(map[string]MethodHandler),
	}
}

// Register 注册方法
func (r *MethodRegistry) Register(method string, handler MethodHandler) {
	r.methods[method] = handler
}

// Call 调用方法
func (r *MethodRegistry) Call(method string, sessionID string, params map[string]interface{}) (interface{}, error) {
	handler, ok := r.methods[method]
	if !ok {
		return nil, fmt.Errorf("method not found: %s", method)
	}
	return handler(sessionID, params)
}

// ParseRequest 解析请求（仅 JSON-RPC 2.0 格式，用于兼容旧客户端）
func ParseRequest(data []byte) (*JSONRPCRequest, error) {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	if req.JSONRPC != "2.0" {
		return nil, fmt.Errorf("unsupported jsonrpc version: %s", req.JSONRPC)
	}
	if req.Method == "" {
		return nil, fmt.Errorf("method is required")
	}
	return &req, nil
}

// GatewayRequest 前端网关请求帧（type: "req"）
type GatewayRequest struct {
	Type   string                 `json:"type"`
	ID     string                 `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// ParseGatewayRequest 解析请求，支持前端格式 type:"req" 与 JSON-RPC 2.0
func ParseGatewayRequest(data []byte) (*JSONRPCRequest, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	req := &JSONRPCRequest{Params: make(map[string]interface{})}
	if t, _ := raw["type"].(string); t == "req" {
		req.ID, _ = raw["id"].(string)
		req.Method, _ = raw["method"].(string)
		if p, ok := raw["params"].(map[string]interface{}); ok {
			req.Params = p
		}
	} else {
		req.JSONRPC, _ = raw["jsonrpc"].(string)
		req.ID, _ = raw["id"].(string)
		req.Method, _ = raw["method"].(string)
		if p, ok := raw["params"].(map[string]interface{}); ok {
			req.Params = p
		}
		if req.JSONRPC != "" && req.JSONRPC != "2.0" {
			return nil, fmt.Errorf("unsupported jsonrpc version: %s", req.JSONRPC)
		}
	}
	if req.Method == "" {
		return nil, fmt.Errorf("method is required")
	}
	return req, nil
}

// GatewayError 前端期望的错误结构（code 为 string）
type GatewayError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// GatewayResponseFrame 前端期望的响应帧（type: "res"）
type GatewayResponseFrame struct {
	Type    string         `json:"type"`
	ID      string         `json:"id"`
	OK      bool           `json:"ok"`
	Payload interface{}    `json:"payload,omitempty"`
	Error   *GatewayError  `json:"error,omitempty"`
}

// NewGatewaySuccess 构造成功响应
func NewGatewaySuccess(id string, payload interface{}) *GatewayResponseFrame {
	return &GatewayResponseFrame{Type: "res", ID: id, OK: true, Payload: payload}
}

// NewGatewayErrorFrame 构造错误响应
func NewGatewayErrorFrame(id string, code string, message string, details interface{}) *GatewayResponseFrame {
	return &GatewayResponseFrame{
		Type:  "res",
		ID:    id,
		OK:    false,
		Error: &GatewayError{Code: code, Message: message, Details: details},
	}
}

// EncodeResponse 编码响应
func EncodeResponse(resp *JSONRPCResponse) ([]byte, error) {
	return json.Marshal(resp)
}
