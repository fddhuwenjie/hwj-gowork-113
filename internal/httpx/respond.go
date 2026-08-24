// Package httpx 提供 HTTP JSON 接口层：路由、中间件、统一错误与分页。
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"germplasm/internal/apperr"
)

// writeJSON 以统一格式输出 JSON。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// writeError 输出统一错误响应：{"error":{"code","message","details"}}。
func writeError(w http.ResponseWriter, err error) {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		writeJSON(w, ae.Status, apperr.BodyOf(err))
		return
	}
	writeJSON(w, http.StatusInternalServerError, apperr.BodyOf(err))
}

// decodeJSON 解析请求体，空体返回 nil。
func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return apperr.Validation("请求体 JSON 非法: " + err.Error())
	}
	return nil
}
