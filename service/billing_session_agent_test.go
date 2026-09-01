package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

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
