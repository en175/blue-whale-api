#!/usr/bin/env python3
"""
修复 options.ModelRatio / ModelPrice 的 4 位截断（IK4BRJ）。

根因:
  ModelRatio/ModelPrice 录入时把 RMB 官方价 ÷汇率(7) ÷2 反算成 USD 倍率,
  截断成 4 位小数存 DB。前端 price.ts 公式 ratio×2×usdExchangeRate 回 RMB
  时丢精度 → ¥7.9996（该 ¥8）、¥10.7995（该 ¥10.8）等浮点尾巴。
  汇率 USDExchangeRate=7，故截断值全是 1/7 倍数（0.5714 / 0.8571 / 0.2286 …）。

修复:
  round(v*14, 2) / 14  —— 反推官方 RMB 价（round 到 2 位）再除 14，
  恢复精确 1/7 倍数（0.57142857… = 4/7），×2×7 回到干净 RMB。

范围:
  仅 ModelRatio / ModelPrice（÷14 截断，反推可靠）。
  CompletionRatio / CacheRatio 跳过 —— 倍率语义不同（6.6667=20/3、0.0833=1/12），
  ÷14 反推会错，需按各自分母单独处理。

用法（DB 凭证不落盘，走 docker exec 中转）:
  # 1. 备份
  docker exec <mysql> -N -e "SELECT value FROM options WHERE \`key\`='ModelRatio'" > mr-backup.json
  # 2. 反推
  python3 fix-ratio-precision.py < mr-backup.json > mr-new.json
  # 3. UPDATE（拼接 SQL，JSON 双引号在 SQL 单引号内安全）
  python3 -c "print(\"UPDATE options SET value='\"+open('mr-new.json').read()+\"' WHERE \\\`key\\\`='ModelRatio';\")" > update.sql
  docker exec -i <mysql> new-api < update.sql

  <mysql> = blue-whale-mysql mysql -h<host> -uroot -p<pwd>
"""
import sys
import json


def fix_value(v):
    """÷14 截断值反推为精确 1/7 倍数；非数值原样返回。"""
    if not isinstance(v, (int, float)):
        return v
    official_rmb = round(v * 14, 2)  # 反推官方 RMB 价（input per 1M tokens）
    return official_rmb / 14         # 精确 ratio = 官方价 / (汇率×2) = 官方价 / 14


def main():
    raw = sys.stdin.read().strip()
    data = json.loads(raw)
    fixed = {k: fix_value(v) for k, v in data.items()}
    # 紧凑 JSON（options.value 存的就是紧凑 JSON）
    sys.stdout.write(json.dumps(fixed, ensure_ascii=False, separators=(',', ':')))

    # stderr 输出变更统计（不污染 stdout JSON）
    changed = sum(
        1 for k, v in data.items()
        if isinstance(v, (int, float)) and abs(fix_value(v) - v) > 1e-9
    )
    sys.stderr.write(f'\n[fix-ratio-precision] {changed}/{len(data)} 改\n')


if __name__ == '__main__':
    main()
