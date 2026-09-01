package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 记账用的资金来源替身：真的那两个（钱包/订阅）都要活着的 DB。
type recordingFunding struct {
	source       string
	preConsumed  int
	settledDelta int
	refundCalls  int
}

func (f *recordingFunding) Source() string         { return f.source }
func (f *recordingFunding) PreConsume(a int) error { f.preConsumed += a; return nil }
func (f *recordingFunding) Settle(delta int) error { f.settledDelta += delta; return nil }
func (f *recordingFunding) Refund() error          { f.refundCalls++; return nil }

// 换掉令牌账本的两个写入口，返回一个记录器和恢复函数。
type ledgerCalls struct{ decrease, increase []int }

func captureLedger(t *testing.T) *ledgerCalls {
	t.Helper()
	calls := &ledgerCalls{}
	origDec, origInc := decreaseTokenQuota, increaseTokenQuota
	decreaseTokenQuota = func(_ int, _ string, quota int) error {
		calls.decrease = append(calls.decrease, quota)
		return nil
	}
	increaseTokenQuota = func(_ int, _ string, quota int) error {
		calls.increase = append(calls.increase, quota)
		return nil
	}
	t.Cleanup(func() { decreaseTokenQuota, increaseTokenQuota = origDec, origInc })
	return calls
}

// 蓝鲸网关代打是 tokenless 的：`PreConsumeTokenQuota` 对它直接返回，没有任何 token 行被扣。
// 但 `tokenConsumed` 仍然被记上（`needsRefundLocked` 拿它当钱包退款的判据），所以动账本的
// 那几处必须自己判 tokenless，否则会拿 TokenId=0 / TokenKey="" 去改账。
//
// 危害不在 DB——`WHERE id = 0` 空转。在 Redis：`cacheIncrTokenQuota("")` 对
// `GenerateHMAC("")` 这个固定值 HINCRBY，每次网关调用都写同一个没有 TTL 的垃圾 hash。
// 这个测试锁的就是「谁该跳过账本」这个判断本身，不去碰 Redis。
func TestSkipsTokenLedger(t *testing.T) {
	cases := []struct {
		name string
		info relaycommon.RelayInfo
		want bool
	}{
		{"普通 token 调用要动账本", relaycommon.RelayInfo{TokenId: 7}, false},
		{"Playground 不动账本", relaycommon.RelayInfo{IsPlayground: true}, true},
		{"网关代打不动账本", relaycommon.RelayInfo{AgentBilling: true}, true},
		{"两者同时为真也不动", relaycommon.RelayInfo{IsPlayground: true, AgentBilling: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := tc.info
			s := &BillingSession{relayInfo: &info}
			if got := s.skipsTokenLedger(); got != tc.want {
				t.Fatalf("skipsTokenLedger() = %v, want %v", got, tc.want)
			}
		})
	}
}

// tokenConsumed 在 tokenless 路上**照样**被赋值，这是有意的：清零会让 needsRefundLocked
// 返回 false，失败时钱包预扣永远不退。这条用例把这个意图钉住，免得有人"顺手"改成条件赋值。
func TestAgentBillingStillNeedsWalletRefund(t *testing.T) {
	info := relaycommon.RelayInfo{AgentBilling: true}
	s := &BillingSession{relayInfo: &info, tokenConsumed: 500}
	if !s.needsRefundLocked() {
		t.Fatal("网关代打失败时钱包预扣必须还能退：needsRefundLocked 应为 true")
	}
	if !s.skipsTokenLedger() {
		t.Fatal("同一次调用又必须跳过令牌账本——两件事互不冲突")
	}
}

// 谓词对了不代表调用点用了它。这两条钉的是调用点本身——上一版只测了谓词，
// 谁把 Settle 里的判断改回 IsPlayground，测试不会红。
func TestSettleSkipsTokenLedgerForAgentBilling(t *testing.T) {
	calls := captureLedger(t)
	funding := &recordingFunding{source: BillingSourceWallet}
	info := relaycommon.RelayInfo{AgentBilling: true}
	s := &BillingSession{relayInfo: &info, funding: funding, preConsumedQuota: 100, tokenConsumed: 100}

	if err := s.Settle(150); err != nil {
		t.Fatalf("Settle 不该出错: %v", err)
	}
	// 资金来源照常结算——跳过的只是令牌账本那一层。
	if funding.settledDelta != 50 {
		t.Fatalf("钱包应结算 delta=50，实际 %d", funding.settledDelta)
	}
	if len(calls.decrease) != 0 || len(calls.increase) != 0 {
		t.Fatalf("网关代打没有 token 行，不该动令牌账本；实际 decrease=%v increase=%v",
			calls.decrease, calls.increase)
	}
}

// 反向对照：同一段代码在普通 token 调用下必须照常改账，否则上面那条用例即使闸被删掉也会绿。
func TestSettleStillAdjustsTokenLedgerForOrdinaryToken(t *testing.T) {
	calls := captureLedger(t)
	funding := &recordingFunding{source: BillingSourceWallet}
	info := relaycommon.RelayInfo{TokenId: 7}
	s := &BillingSession{relayInfo: &info, funding: funding, preConsumedQuota: 100, tokenConsumed: 100}

	if err := s.Settle(150); err != nil {
		t.Fatalf("Settle 不该出错: %v", err)
	}
	if len(calls.decrease) != 1 || calls.decrease[0] != 50 {
		t.Fatalf("普通 token 调用必须扣令牌额度 50，实际 %v", calls.decrease)
	}
}

// Refund 走 gopool 的 goroutine，所以这里要等它跑完再断言。
func TestRefundSkipsTokenLedgerForAgentBillingButStillRefundsWallet(t *testing.T) {
	calls := captureLedger(t)
	done := make(chan struct{})
	origInc := increaseTokenQuota
	increaseTokenQuota = func(id int, key string, quota int) error {
		defer close(done)
		return origInc(id, key, quota)
	}
	funding := &recordingFunding{source: BillingSourceWallet}
	info := relaycommon.RelayInfo{AgentBilling: true}
	s := &BillingSession{relayInfo: &info, funding: funding, preConsumedQuota: 100, tokenConsumed: 100}

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	s.Refund(ctx)
	waitFor(t, func() bool { return funding.refundCalls == 1 }, "钱包退款没有发生")

	select {
	case <-done:
		t.Fatal("网关代打不该退令牌额度——它压根没扣过")
	default:
	}
	if len(calls.increase) != 0 {
		t.Fatalf("不该动令牌账本，实际 increase=%v", calls.increase)
	}
}

func TestRefundStillRefundsTokenLedgerForOrdinaryToken(t *testing.T) {
	calls := captureLedger(t)
	funding := &recordingFunding{source: BillingSourceWallet}
	info := relaycommon.RelayInfo{TokenId: 7}
	s := &BillingSession{relayInfo: &info, funding: funding, preConsumedQuota: 100, tokenConsumed: 100}

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	s.Refund(ctx)
	waitFor(t, func() bool { return len(calls.increase) == 1 }, "普通 token 调用必须退还令牌额度")
	if calls.increase[0] != 100 {
		t.Fatalf("应退还 100，实际 %v", calls.increase)
	}
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}
