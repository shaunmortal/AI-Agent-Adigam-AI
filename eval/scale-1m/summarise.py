"""Prints the markdown tables for results/summary.md from the run JSON files (numbers are copied,
formatted to 1 decimal for ms and req/s; the JSON keeps microsecond precision)."""
import json, os, re

R = os.path.join(os.path.dirname(os.path.abspath(__file__)), "results")
RUNS = [  # label, pool size, series
    ("pass1-warmup", 12, "pass 1 warm-up"), ("pass1-c32", 12, "pass 1"), ("pass1-c64", 12, "pass 1"),
    ("pass1-c128", 12, "pass 1"), ("pass1-c256", 12, "pass 1"),
    ("pass2-warmup", 12, "pass 2 warm-up"), ("pass2-c32", 12, "pass 2"), ("pass2-c64", 12, "pass 2"),
    ("pass2-c128", 12, "pass 2"), ("pass2-c256", 12, "pass 2"),
    ("pass3-warmup", 12, "pass 3 warm-up"), ("pass3-c256", 12, "pass 3"),
    ("pool40-warmup", 40, "pool 40 warm-up"), ("pool40-c64", 40, "pool 40"), ("pool40-c256", 40, "pool 40"),
    ("pool40-rerun-c256", 40, "pool 40 rerun"),
]


def load(label):
    with open(os.path.join(R, label + ".json")) as f:
        rep = json.load(f)
    with open(os.path.join(R, label + ".meta.json")) as f:
        meta = json.load(f)
    return rep, meta


def la1(s):
    return s.strip("{} ").split()[0]


def errs(responses):
    # pool40-c256 ran before scaleload grouped transport errors; strip the addresses here
    bad = {}
    for k, v in responses.items():
        if re.fullmatch(r"20[01]", k):
            continue
        k = re.sub(r"transport: (dial|read|write) tcp \S+?(->\S+?)?: ", r"transport: \1: ", k)
        bad[k] = bad.get(k, 0) + v
    return ", ".join(f"{k}: {v}" for k, v in sorted(bad.items())) or "none"


def f1(x):
    return f"{x:,.1f}"


def overall():
    print("| run | pool | c | duration s (elapsed) | requests | req/s | p50 ms | p95 ms | p99 ms | max ms | errors | 1-min load avg before | load avg during (min-max) | DB commits/s | DB cache hit % | DB read wait s |")
    print("|---|---:|---:|---|---:|---:|---:|---:|---:|---:|---|---:|---|---:|---:|---:|")
    for label, pool, _ in RUNS:
        rep, meta = load(label)
        o, d = rep["overall"], meta["pg_delta"]
        during = [float(la1(s["loadavg"])) for s in meta.get("during", [])]
        dr = f"{min(during):.2f}-{max(during):.2f}" if during else "not sampled"
        hit = 100 * d["blks_hit"] / (d["blks_hit"] + d["blks_read"])
        print(f"| {label} | {pool} | {rep['concurrency']} | {rep['durationS']:.0f} ({rep['elapsedS']:.3f}) | {o['requests']:,} | {f1(o['rps'])} "
              f"| {f1(o['p50Ms'])} | {f1(o['p95Ms'])} | {f1(o['p99Ms'])} | {f1(o['maxMs'])} | {o['errors']:,} ({errs(o['responses'])}) "
              f"| {la1(meta['before']['loadavg_1_5_15'])} | {dr} | {d['xact_commit'] / rep['elapsedS']:,.1f} | {hit:.2f} | {d['blk_read_time'] / 1000:,.1f} |")


def endpoints(labels):
    order = [e["name"] for e in load(labels[0])[0]["mix"]]
    for label in labels:
        rep, _ = load(label)
        print(f"\n**{label}** (c={rep['concurrency']}, {rep['elapsedS']:.3f} s)\n")
        print("| endpoint | weight % | requests | req/s | p50 ms | p95 ms | p99 ms | max ms | errors |")
        print("|---|---:|---:|---:|---:|---:|---:|---:|---|")
        weights = {e["name"]: e["weight"] for e in rep["mix"]}
        for n in order:
            s = rep["endpoints"][n]
            print(f"| {n} | {weights[n]} | {s['requests']:,} | {f1(s['rps'])} | {f1(s['p50Ms'])} | {f1(s['p95Ms'])} "
                  f"| {f1(s['p99Ms'])} | {f1(s['maxMs'])} | {s['errors']} |")
        o = rep["overall"]
        print(f"| **all** | 100 | {o['requests']:,} | {f1(o['rps'])} | {f1(o['p50Ms'])} | {f1(o['p95Ms'])} | {f1(o['p99Ms'])} | {f1(o['maxMs'])} | {o['errors']} |")


def cpu():
    print("| run | samples | API % | Postgres (this cluster) % | load generator % | stub auth % | Redis % | everything else % |")
    print("|---|---:|---:|---:|---:|---:|---:|---:|")
    for label, _, _ in RUNS:
        _, meta = load(label)
        ss = meta.get("during", [])
        if not ss:
            continue
        def avg(k):
            return sum(s["stack_cpu_pct"].get(k, 0) for s in ss) / len(ss)
        other = sum(s["other_cpu_pct_total"] for s in ss) / len(ss)
        pg = f"{avg('postgres'):.1f}" if "redis" in ss[0]["stack_cpu_pct"] else "not separated (in 'else')"
        rd = f"{avg('redis'):.1f}" if "redis" in ss[0]["stack_cpu_pct"] else "in 'else'"
        print(f"| {label} | {len(ss)} | {avg('api'):.1f} | {pg} | {avg('loadgen'):.1f} | {avg('stubauth'):.1f} | {rd} | {other:.1f} |")


def answers():
    total = 0
    for label, _, _ in RUNS:
        total += load(label)[0]["endpoints"]["POST /attempts"]["responses"].get("201", 0)
    return total


if __name__ == "__main__":
    print("## overall\n"); overall()
    print("\n## cpu\n"); cpu()
    print("\n## endpoints"); endpoints([l for l, _, s in RUNS if s in ("pass 1", "pass 2", "pass 3", "pool 40", "pool 40 rerun")])
    print(f"\nanswers recorded (201) across these runs: {answers():,}")
