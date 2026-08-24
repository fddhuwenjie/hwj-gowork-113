// Package clock 提供可注入时钟，便于测试中对时间进行控制。
package clock

import (
	"sync"
	"time"
)

// Clock 抽象时间来源，生产环境使用 Real，测试使用 Fake。
type Clock interface {
	Now() time.Time
}

// Real 返回真实系统时间（UTC）。
type Real struct{}

// Now 返回当前 UTC 时间。
func (Real) Now() time.Time { return time.Now().UTC() }

// Fake 为可手动推进的假时钟，仅用于测试。
type Fake struct {
	mu sync.Mutex
	t  time.Time
}

// NewFake 创建起始时间为 t 的假时钟。
func NewFake(t time.Time) *Fake { return &Fake{t: t.UTC()} }

// Now 返回假时钟当前时间。
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

// Advance 将假时钟向前推进 d。
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

// Set 直接设置假时钟时间。
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = t.UTC()
}

// Format 将时间序列化为 SQLite 存储使用的 RFC3339Nano 字符串。
func Format(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// Parse 解析 SQLite 中的时间字符串。
func Parse(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

// MustParse 解析时间字符串，失败时 panic，仅用于测试。
func MustParse(s string) time.Time {
	t, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return t
}
