package httpx

import (
	"context"
	"net/http"
	"strings"
)

// pathValuesKey 为请求上下文中路径参数的键。
type pathValuesKey struct{}

// route 为一条注册路由：方法 + 分段模式（{name} 为路径参数）。
type route struct {
	method string
	segs   []string
	h      http.HandlerFunc
}

// router 为自实现的轻量路由器，兼容 go1.21 语言版本，
// 支持 "/api/v1/resources/{id}" 形式的路径参数。
type router struct {
	routes []route
}

// Handle 注册路由。
func (rt *router) Handle(method, pattern string, h http.HandlerFunc) {
	segs := splitPath(pattern)
	rt.routes = append(rt.routes, route{method: method, segs: segs, h: h})
}

// splitPath 将路径切分为非空段。
func splitPath(p string) []string {
	parts := strings.Split(p, "/")
	var segs []string
	for _, s := range parts {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return segs
}

// ServeHTTP 匹配路由并注入路径参数。
func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pathSegs := splitPath(r.URL.Path)
	methodMismatch := false
	for _, candidate := range rt.routes {
		values, ok := matchSegments(candidate.segs, pathSegs)
		if !ok {
			continue
		}
		if candidate.method != r.Method {
			methodMismatch = true
			continue
		}
		if len(values) > 0 {
			r = r.WithContext(context.WithValue(r.Context(), pathValuesKey{}, values))
		}
		candidate.h(w, r)
		return
	}
	if methodMismatch {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error": map[string]any{"code": "METHOD_NOT_ALLOWED", "message": "方法不允许"},
		})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{
		"error": map[string]any{"code": "NOT_FOUND", "message": "接口不存在"},
	})
}

// matchSegments 逐段匹配，{name} 段捕获路径参数。
func matchSegments(pattern, path []string) (map[string]string, bool) {
	if len(pattern) != len(path) {
		return nil, false
	}
	values := map[string]string{}
	for i, seg := range pattern {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			values[seg[1:len(seg)-1]] = path[i]
			continue
		}
		if seg != path[i] {
			return nil, false
		}
	}
	return values, true
}

// pathValue 读取路径参数。
func pathValue(r *http.Request, name string) string {
	values, _ := r.Context().Value(pathValuesKey{}).(map[string]string)
	return values[name]
}

// pathID 读取最常用的 id 路径参数。
func pathID(r *http.Request) string { return pathValue(r, "id") }
