package httpx

import (
	"net/http"
	"strconv"
)

// pageParams 解析稳定分页参数：cursor 为上一页返回的 next_cursor，limit 默认 20、最大 200。
func pageParams(r *http.Request) (cursor string, limit int) {
	cursor = r.URL.Query().Get("cursor")
	limit = 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	return cursor, limit
}

// queryParam 读取可选查询参数。
func queryParam(r *http.Request, name string) string {
	return r.URL.Query().Get(name)
}

// versionParam 从请求体或查询参数读取乐观锁版本。
type versionBody struct {
	Version int64 `json:"version"`
}
