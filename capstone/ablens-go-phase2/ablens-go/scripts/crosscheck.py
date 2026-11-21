import csv, json, math, sys
from datetime import datetime
import numpy as np
from scipy import stats

data_path, report_path = sys.argv[1], sys.argv[2]
rep = json.load(open(report_path))

def ok_ts(s):
    try:
        datetime.fromisoformat(s.replace("Z", "+00:00")); return True
    except ValueError:
        return False

rows, seen = [], set()
counts = dict(total=0, malformed=0, unknown=0, dup=0)
with open(data_path, newline="") as f:
    rd = csv.reader(f)
    next(rd)
    for rec in rd:
        counts["total"] += 1
        try:
            if len(rec) < 6: raise ValueError
            eid, uid, var, ts, conv, rev = rec[:6]
            if not ok_ts(ts): raise ValueError
            conv = int(conv)
            rev = float(rev) if rev != "" else 0.0
            if not eid or not uid or conv not in (0, 1) or not math.isfinite(rev) or rev < 0: raise ValueError
        except ValueError:
            counts["malformed"] += 1; continue
        if var not in ("control", "treatment"):
            counts["unknown"] += 1; continue
        if eid in seen:
            counts["dup"] += 1; continue
        seen.add(eid)
        rows.append((uid, var, conv, rev))

first = {}
bad_users = set()
for uid, var, *_ in rows:
    if uid in first and first[uid] != var: bad_users.add(uid)
    first.setdefault(uid, var)
clean = [r for r in rows if r[0] not in bad_users]

q = rep["data_quality"]
checks = []
def chk(name, got, want, tol=1e-9):
    good = abs(got - want) <= tol * max(1.0, abs(want))
    checks.append(good)
    print(f"{'PASS' if good else 'FAIL'}  {name}: go={got!r} scipy/python={want!r}")

chk("total rows", q["total_rows"], counts["total"], 0)
chk("malformed", q["malformed_rows"], counts["malformed"], 0)
chk("unknown variant", q["unknown_variant_rows"], counts["unknown"], 0)
chk("duplicates", q["duplicate_event_rows"], counts["dup"], 0)
chk("cross-variant users", q["cross_variant_users"], len(bad_users), 0)
chk("valid rows", q["valid_rows"], len(clean), 0)

c = [r for r in clean if r[1] == "control"]; t = [r for r in clean if r[1] == "treatment"]
obs = [len(c), len(t)]; tot = sum(obs)
s = stats.chisquare(obs, [tot * 0.5, tot * 0.5])
chk("SRM chi-square", q["srm"]["chi_square"], s.statistic, 1e-9)
chk("SRM p-value", q["srm"]["p_value"], s.pvalue, 1e-7)

for m in rep["metrics"]:
    name = m["metric"]; i = 2 if name == "converted" else 3
    a = np.array([r[i] for r in c], float); b = np.array([r[i] for r in t], float)
    chk(f"{name} control mean", m["control"]["mean"], a.mean(), 1e-9)
    chk(f"{name} treatment mean", m["treatment"]["mean"], b.mean(), 1e-9)
    chk(f"{name} control variance", m["control"]["variance"], a.var(ddof=1), 1e-9)
    if name == "converted":
        p1, p2 = a.mean(), b.mean(); pool = (a.sum() + b.sum()) / (len(a) + len(b))
        se = math.sqrt(pool * (1 - pool) * (1 / len(a) + 1 / len(b))); z = (p2 - p1) / se
        chk("z statistic", m["test"]["statistic"], z, 1e-9)
        chk("z p-value", m["test"]["p_value"], 2 * stats.norm.sf(abs(z)), 1e-7)
    else:
        r = stats.ttest_ind(b, a, equal_var=False)
        chk("Welch t", m["test"]["statistic"], r.statistic, 1e-9)
        chk("Welch p-value", m["test"]["p_value"], r.pvalue, 1e-7)
        ci = r.confidence_interval(0.95)
        chk("Welch CI low", m["test"]["ci_low"], ci.low, 1e-7)
        chk("Welch CI high", m["test"]["ci_high"], ci.high, 1e-7)
    bs = m.get("bootstrap")
    if bs:
        w = m["test"]["ci_high"] - m["test"]["ci_low"]
        near = abs(bs["ci_low"] - m["test"]["ci_low"]) < 0.25 * w and abs(bs["ci_high"] - m["test"]["ci_high"]) < 0.25 * w
        checks.append(near)
        print(f"{'PASS' if near else 'FAIL'}  {name} bootstrap CI [{bs['ci_low']:.5g}, {bs['ci_high']:.5g}] agrees with analytic [{m['test']['ci_low']:.5g}, {m['test']['ci_high']:.5g}]")

print(f"\n{sum(checks)}/{len(checks)} checks passed")
sys.exit(0 if all(checks) else 1)
