// Package apperr 定义统一应用错误及 HTTP 映射。
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// 错误码常量，统一错误响应体中的 code 字段。
const (
	CodeValidation     = "VALIDATION"           // 请求参数校验失败
	CodeNotFound       = "NOT_FOUND"            // 资源不存在
	CodeConflict       = "CONFLICT"             // 唯一性等冲突
	CodeStateConflict  = "STATE_CONFLICT"       // 状态机不允许的转换
	CodeOptimisticLock = "OPTIMISTIC_LOCK"      // 乐观锁版本不匹配
	CodeIdempotency    = "IDEMPOTENCY_CONFLICT" // 幂等键与原始请求不一致
	CodeQuantity       = "QUANTITY_VIOLATION"   // 数量守恒被破坏或库存不足
	CodeQuality        = "QUALITY_VIOLATION"    // 检测覆盖不足或纯度不合格
	CodeWindow         = "ENV_WINDOW_VIOLATION" // 环境时间窗覆盖不足
	CodeInternal       = "INTERNAL"             // 内部错误
)

// Error 为带错误码与 HTTP 状态的应用错误。
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
	Status  int    `json:"-"`
}

// Error 实现 error 接口。
func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// New 创建应用错误。
func New(status int, code, msg string) *Error {
	return &Error{Code: code, Message: msg, Status: status}
}

// Newf 按格式创建应用错误。
func Newf(status int, code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Status: status}
}

// WithDetails 附加错误详情。
func (e *Error) WithDetails(d any) *Error { e.Details = d; return e }

// Validation 参数错误。
func Validation(msg string) *Error { return New(http.StatusBadRequest, CodeValidation, msg) }

// Validationf 格式化参数错误。
func Validationf(format string, args ...any) *Error {
	return Newf(http.StatusBadRequest, CodeValidation, format, args...)
}

// NotFound 资源不存在。
func NotFound(entity, id string) *Error {
	return Newf(http.StatusNotFound, CodeNotFound, "%s 不存在: %s", entity, id)
}

// Conflict 冲突错误。
func Conflict(msg string) *Error { return New(http.StatusConflict, CodeConflict, msg) }

// Statef 状态机冲突。
func Statef(format string, args ...any) *Error {
	return Newf(http.StatusConflict, CodeStateConflict, format, args...)
}

// OptimisticLock 乐观锁冲突。
func OptimisticLock(entity, id string) *Error {
	return Newf(http.StatusConflict, CodeOptimisticLock, "%s %s 版本不匹配，请刷新后重试", entity, id)
}

// Idempotency 幂等冲突。
func Idempotency(key string) *Error {
	return Newf(http.StatusConflict, CodeIdempotency, "幂等键 %s 已用于不同的请求体", key)
}

// Quantity 数量守恒错误。
func Quantity(msg string) *Error { return New(http.StatusConflict, CodeQuantity, msg) }

// Quantityf 格式化数量守恒错误。
func Quantityf(format string, args ...any) *Error {
	return Newf(http.StatusConflict, CodeQuantity, format, args...)
}

// Quality 质量约束错误。
func Quality(msg string) *Error { return New(http.StatusConflict, CodeQuality, msg) }

// Window 环境时间窗错误。
func Window(msg string) *Error { return New(http.StatusConflict, CodeWindow, msg) }

// Internal 内部错误。
func Internal(err error) *Error {
	return New(http.StatusInternalServerError, CodeInternal, "内部错误").WithDetails(err.Error())
}

// StatusOf 返回 error 对应的 HTTP 状态码。
func StatusOf(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Status
	}
	return http.StatusInternalServerError
}

// BodyOf 将 error 转换为统一错误响应体。
func BodyOf(err error) map[string]any {
	var e *Error
	if errors.As(err, &e) {
		return map[string]any{"error": e}
	}
	return map[string]any{"error": &Error{Code: CodeInternal, Message: "内部错误", Status: 500}}
}
