package billingexpr

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/tidwall/gjson"
)

// RunExpr compiles (with cache) and executes an expression string.
// The environment exposes:
//   - p, c             — prompt / completion tokens (auto-excluding separately-priced sub-categories)
//   - len              — total input context length for tier conditions (never reduced by sub-category exclusion)
//   - cr, cc, cc1h     — cache read / creation / creation-1h tokens
//   - tier(name, value) — trace callback that records which tier matched
//   - max, min, abs, ceil, floor — standard math helpers
//
// Returns the resulting float64 quota (before group ratio) and a TraceResult
// with side-channel info captured by tier() during execution.
func RunExpr(exprStr string, params TokenParams) (float64, TraceResult, error) {
	return RunExprWithRequest(exprStr, params, RequestInput{})
}

func RunExprWithRequest(exprStr string, params TokenParams, request RequestInput) (float64, TraceResult, error) {
	prog, err := CompileFromCache(exprStr)
	if err != nil {
		return 0, TraceResult{}, err
	}
	return runProgram(prog, params, request)
}

// RunExprByHash is like RunExpr but accepts a pre-computed hash for the cache
// lookup, avoiding a redundant SHA-256 computation when the caller already
// holds BillingSnapshot.ExprHash.
func RunExprByHash(exprStr, hash string, params TokenParams) (float64, TraceResult, error) {
	return RunExprByHashWithRequest(exprStr, hash, params, RequestInput{})
}

func RunExprByHashWithRequest(exprStr, hash string, params TokenParams, request RequestInput) (float64, TraceResult, error) {
	prog, err := CompileFromCacheByHash(exprStr, hash)
	if err != nil {
		return 0, TraceResult{}, err
	}
	return runProgram(prog, params, request)
}

// numericArg converts any Go numeric value to float64. The expr vm calls env
// functions through reflection without argument conversion, so integer operands
// (e.g. a `param("n") ?? 1` fallback) used to abort billing with
// "reflect: Call using int as type float64"; accepting all numeric types keeps
// those expressions working.
func numericArg(value interface{}) (float64, error) {
	switch n := value.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int8:
		return float64(n), nil
	case int16:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case uint:
		return float64(n), nil
	case uint8:
		return float64(n), nil
	case uint16:
		return float64(n), nil
	case uint32:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("expected a number, got %T", value)
	}
}

// Tolerant math helpers shared by the compile-time prototype (compile.go) and
// the runtime environment below; their Go types must stay identical in both
// places because the expr vm matches env functions against the compiled
// program's expectations.
func mathMax(a, b interface{}) (float64, error) {
	x, err := numericArg(a)
	if err != nil {
		return 0, fmt.Errorf("max: %w", err)
	}
	y, err := numericArg(b)
	if err != nil {
		return 0, fmt.Errorf("max: %w", err)
	}
	return math.Max(x, y), nil
}

func mathMin(a, b interface{}) (float64, error) {
	x, err := numericArg(a)
	if err != nil {
		return 0, fmt.Errorf("min: %w", err)
	}
	y, err := numericArg(b)
	if err != nil {
		return 0, fmt.Errorf("min: %w", err)
	}
	return math.Min(x, y), nil
}

func mathAbs(value interface{}) (float64, error) {
	v, err := numericArg(value)
	if err != nil {
		return 0, fmt.Errorf("abs: %w", err)
	}
	return math.Abs(v), nil
}

func mathCeil(value interface{}) (float64, error) {
	v, err := numericArg(value)
	if err != nil {
		return 0, fmt.Errorf("ceil: %w", err)
	}
	return math.Ceil(v), nil
}

func mathFloor(value interface{}) (float64, error) {
	v, err := numericArg(value)
	if err != nil {
		return 0, fmt.Errorf("floor: %w", err)
	}
	return math.Floor(v), nil
}

func runProgram(prog *vm.Program, params TokenParams, request RequestInput) (float64, TraceResult, error) {
	trace := TraceResult{}
	headers := normalizeHeaders(request.Headers)

	env := map[string]interface{}{
		"p":     params.P,
		"c":     params.C,
		"len":   params.Len,
		"cr":    params.CR,
		"cc":    params.CC,
		"cc1h":  params.CC1h,
		"img":   params.Img,
		"img_o": params.ImgO,
		"ai":    params.AI,
		"ao":    params.AO,
		"tier": func(name string, value interface{}) (float64, error) {
			v, err := numericArg(value)
			if err != nil {
				return 0, fmt.Errorf("tier(%s): %w", name, err)
			}
			trace.MatchedTier = name
			trace.Cost = v
			return v, nil
		},
		"header": func(key string) string {
			return headers[strings.ToLower(strings.TrimSpace(key))]
		},
		"param": func(path string) interface{} {
			path = strings.TrimSpace(path)
			if path == "" || len(request.Body) == 0 {
				return nil
			}
			result := gjson.GetBytes(request.Body, path)
			if !result.Exists() {
				return nil
			}
			return result.Value()
		},
		"has": func(source interface{}, substr string) bool {
			if source == nil || substr == "" {
				return false
			}
			return strings.Contains(fmt.Sprint(source), substr)
		},
		"hour":    func(tz string) int { return timeInZone(tz).Hour() },
		"minute":  func(tz string) int { return timeInZone(tz).Minute() },
		"weekday": func(tz string) int { return int(timeInZone(tz).Weekday()) },
		"month":   func(tz string) int { return int(timeInZone(tz).Month()) },
		"day":     func(tz string) int { return timeInZone(tz).Day() },
		"max":     mathMax,
		"min":     mathMin,
		"abs":     mathAbs,
		"ceil":    mathCeil,
		"floor":   mathFloor,
	}

	out, err := expr.Run(prog, env)
	if err != nil {
		return 0, trace, fmt.Errorf("expr run error: %w", err)
	}
	f, ok := out.(float64)
	if !ok {
		return 0, trace, fmt.Errorf("expr result is %T, want float64", out)
	}
	return f, trace, nil
}

func timeInZone(tz string) time.Time {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return time.Now().UTC()
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Now().UTC()
	}
	return time.Now().In(loc)
}

func normalizeHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	normalized := make(map[string]string, len(headers))
	for key, value := range headers {
		k := strings.ToLower(strings.TrimSpace(key))
		v := strings.TrimSpace(value)
		if k == "" || v == "" {
			continue
		}
		normalized[k] = v
	}
	return normalized
}
