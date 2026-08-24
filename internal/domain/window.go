package domain

import "time"

// EnvWindowReport 为环境时间窗覆盖评估结果。
type EnvWindowReport struct {
	WindowStart     time.Time `json:"window_start"`
	WindowEnd       time.Time `json:"window_end"`
	BucketHours     int       `json:"bucket_hours"`       // 窗口总小时桶数
	CoveredBuckets  int       `json:"covered_buckets"`    // 有读数且全部达标的小时桶数
	OutOfRangeCount int       `json:"out_of_range_count"` // 越限读数数量
	Coverage        float64   `json:"coverage"`           // 覆盖率 0~1
	OK              bool      `json:"ok"`                 // 是否满足规则要求
}

// EvaluateEnvWindow 评估某个冷库在 [eventAt-beforeHours, eventAt+afterHours]
// 窗口内的温湿度覆盖情况。窗口按小时分桶，桶内存在读数且所有读数
// 均在规则阈值内，该桶记为已覆盖；任何越限读数都会使评估失败。
// 温度与湿度两类传感器读数必须同时满足覆盖率要求。
func EvaluateEnvWindow(rule *RuleVersion, eventAt time.Time, tempReadings, humReadings []SensorReading) EnvWindowReport {
	start := eventAt.Add(-time.Duration(rule.WindowBeforeHours) * time.Hour)
	end := eventAt.Add(time.Duration(rule.WindowAfterHours) * time.Hour)
	buckets := int(end.Sub(start).Hours())
	if buckets <= 0 {
		return EnvWindowReport{WindowStart: start, WindowEnd: end, OK: false}
	}
	tempCovered, tempOut := bucketCoverage(start, buckets, tempReadings, rule.TempInRange)
	humCovered, humOut := bucketCoverage(start, buckets, humReadings, rule.HumidityInRange)
	covered := tempCovered
	if humCovered < covered {
		covered = humCovered
	}
	coverage := float64(covered) / float64(buckets)
	out := tempOut + humOut
	return EnvWindowReport{
		WindowStart:     start,
		WindowEnd:       end,
		BucketHours:     buckets,
		CoveredBuckets:  covered,
		OutOfRangeCount: out,
		Coverage:        coverage,
		OK:              out == 0 && coverage >= rule.MinCoverage,
	}
}

// bucketCoverage 统计读数在小时桶上的覆盖桶数与越限读数数量。
func bucketCoverage(start time.Time, buckets int, readings []SensorReading, inRange func(float64) bool) (covered, outOfRange int) {
	ok := make([]bool, buckets)
	bad := make([]bool, buckets)
	for _, r := range readings {
		idx := int(r.RecordedAt.Sub(start).Hours())
		if idx < 0 || idx >= buckets {
			continue
		}
		if inRange(r.Value) {
			ok[idx] = true
		} else {
			bad[idx] = true
			outOfRange++
		}
	}
	for i := 0; i < buckets; i++ {
		if ok[i] && !bad[i] {
			covered++
		}
	}
	return covered, outOfRange
}

// JudgeVerdict 依据规则对纯度检测进行质量判定。
// 检测覆盖率或纯度合格率低于门槛即判定不合格。
func JudgeVerdict(rule *RuleVersion, coverageRatio, purityRate float64) TestVerdict {
	if coverageRatio < rule.MinCoverage || purityRate < rule.MinPurity {
		return VerdictFail
	}
	return VerdictPass
}

// ConsecutiveDeclines 返回观察序列中连续下降的最长长度。
// 用于识别连续发芽率下降风险；序列按观察时间升序给出。
func ConsecutiveDeclines(rates []float64) int {
	longest, cur := 0, 0
	for i := 1; i < len(rates); i++ {
		if rates[i] < rates[i-1] {
			cur++
			if cur > longest {
				longest = cur
			}
		} else {
			cur = 0
		}
	}
	return longest
}
